// Package invoice raises the official invoice for an approved service order.
//
// SLPA used to report the invoice through a webhook the trader's step waited
// for. It is asked for directly instead: the Single Window knows when the
// accountant has approved the order, so waiting to be told what can be
// requested left the trader in front of an empty panel for as long as the CMS
// took to call. Payment is still reported by webhook — that one is theirs to
// announce, since the money moves outside the Single Window.
package invoice

import (
	"strings"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
)

// GenerateInterpreter drives the call that issues the invoice: POST the service
// order, read back what the trader pays against.
type GenerateInterpreter struct{}

// NewGenerateInterpreter returns the invoice-generation interpreter.
func NewGenerateInterpreter() *GenerateInterpreter { return &GenerateInterpreter{} }

// BuildRequest sends nothing. The service order is named in the path and the
// company by the client key, so there is nothing left for a body to carry — as
// SLPA's own published call shows, which posts an empty one.
func (i *GenerateInterpreter) BuildRequest(map[string]any) remote.Body { return nil }

// BuildHeaders presents the client key the CMS identifies the company by.
func (i *GenerateInterpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa invoice")
}

// Interpret reports whether the CMS issued the invoice and records what the
// trader needs to pay it.
//
// Only the rupee figures are kept. The CMS prices the order in dollars and
// converts, but what is transferred to SLPA's account is the rupee amount, and
// showing both invites a trader to pay the wrong one. The exchange rate is kept
// so the conversion on the invoice document can be checked against the panel.
func (i *GenerateInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	out := map[string]any{}

	details := detailsOf(body)
	number := firstOf(cms.String(body, "invoice_no"), cms.String(details, "invoice_no"))

	// The CMS answers an order it has already invoiced with that same invoice
	// rather than an error, so a repeated call is not a failure: the number is
	// what says an invoice exists.
	issued := callErr == nil && !cms.HasErrors(body) && number != ""
	if !issued {
		out["error"] = describeFailure(callErr, body)
		return false, out
	}

	out["invoice_no"] = number
	out["service_order_no"] = cms.String(body, "service_order_no")
	out["cms_status"] = firstOf(cms.String(details, "status"), cms.String(body, "so_status"))

	// Recorded only when the CMS sent them: a panel showing an empty serial or a
	// zero payable reads as a fact about the invoice rather than a gap in the
	// answer.
	put(out, "invoice_serial", cms.String(details, "invoice_serial"))
	put(out, "generated_at", cms.String(details, "invoice_generated_at"))
	put(out, "payment_slip_url", cms.String(details, "payment_slip_url"))
	put(out, "invoice_url", cms.String(details, "invoice_url"))

	if payable, ok := number64(details, "total_payable_lkr", "total_lkr"); ok {
		out["payable_lkr"] = payable
	}
	if rate, ok := number64(details, "exchange_rate"); ok {
		out["exchange_rate"] = rate
	}

	// The order is invoiced but not yet paid, and the step that follows waits for
	// SLPA to say it has been. Stated here so the workflow's gateway reads one
	// value whether it arrives from this call or from the payment webhook.
	out["paid"] = paidFlag(body, details)
	return true, out
}

// detailsOf reads the block the CMS puts the invoice itself in. A missing block
// is an empty one, so every read below answers "" or zero rather than panicking
// on an answer shaped differently than expected.
func detailsOf(body map[string]any) map[string]any {
	details, _ := body["details"].(map[string]any)
	if details == nil {
		return map[string]any{}
	}
	return details
}

// paidFlag reads whether the CMS considers the invoice settled already. It
// normally is not — the invoice has just been raised — but an order invoiced and
// paid before this call ran must not send the trader to wait for a payment that
// has happened.
func paidFlag(body, details map[string]any) bool {
	if paid, ok := body["is_paid"].(bool); ok && paid {
		return true
	}
	return strings.TrimSpace(cms.String(details, "invoice_paid_at")) != ""
}

// number64 reads the first of keys the CMS sent as a number, reporting whether
// any of them was there. JSON numbers arrive as float64; an amount sent as a
// string is read too, since the CMS has been seen to send both.
func number64(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		case string:
			if f, err := parseFloat(v); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// put records a value only when the CMS sent one.
func put(out map[string]any, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		out[key] = v
	}
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// describeFailure builds the trader-facing message for an invoice the CMS did
// not issue, preferring its own reasons over anything invented here.
//
// This endpoint states a refusal in a bare "message" on the envelope as well as
// in the "error" object every other SLPA endpoint uses, so the envelope is read
// too — cms.Failure knows only the object, and falling through to the generic
// wording would drop the one sentence that says why.
func describeFailure(callErr error, body map[string]any) string {
	const intro = "SLPA did not issue the invoice for this service order:"
	const outro = "\n\nThe service order is approved, so this can be tried again in a few minutes."

	if len(cms.Reasons(body)) == 0 {
		if message := strings.TrimSpace(cms.String(body, "message")); message != "" {
			return intro + "\n\n- " + message + outro
		}
	}
	return cms.Failure(callErr, body, intro, outro)
}
