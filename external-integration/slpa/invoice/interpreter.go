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
// Every field is read from the one place the CMS documents it. An earlier
// version tried several keys per value, which hid a real fault: the payment
// slip link lives on the payment_slip block, not beside it, so the fallback
// quietly produced nothing and the trader was shown an invoice with no way to
// pay it.
//
// Only the rupee figures are kept. The CMS prices the order in dollars and
// converts, but what is transferred to SLPA's account is the rupee amount, and
// showing both invites a trader to pay the wrong one. The exchange rate is kept
// so the conversion on the invoice document can be checked against the panel.
func (i *GenerateInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	details := mapAt(body, "details")
	number := cms.String(details, "invoice_no")

	// The CMS answers an order it has already invoiced with that same invoice
	// rather than an error, so a repeated call is not a failure: the number is
	// what says an invoice exists.
	if callErr != nil || cms.HasErrors(body) || number == "" {
		return false, map[string]any{"error": describeFailure(callErr, body)}
	}

	slip := mapAt(details, "payment_slip")
	paid, _ := details["is_paid"].(bool)

	out := map[string]any{
		"invoice_no":       number,
		"service_order_no": cms.String(body, "service_order_no"),
		"cms_status":       cms.String(details, "status"),
		"items":            lineItems(details),
		"payment_slip":     paymentSlip(slip),

		// Whether the order is already settled, which the step that follows
		// gates on: an invoice raised against a payment that has already landed
		// must not park the trader waiting for one.
		//
		// Read from the CMS's own flag rather than inferred from
		// invoice_paid_at. The timestamp is null on an unpaid invoice, so it
		// happens to agree, but it answers "when" -- an answer that is already
		// paid and states is_paid without a timestamp would be read as unpaid
		// and wait forever.
		"paid": paid,
	}

	// Recorded only when the CMS sent them. A panel showing an empty link or a
	// zero payable states a fact about the invoice rather than a gap in the
	// answer: "" behind a download reads as a document that failed, and LKR
	// 0.00 as nothing to pay.
	if v := cms.String(details, "invoice_serial"); v != "" {
		out["invoice_serial"] = v
	}
	if v := cms.String(details, "invoice_generated_at"); v != "" {
		out["generated_at"] = v
	}
	// The one link the trader acts on. The invoice document is not offered
	// beside it: two links, one of which cannot be paid against, is how a
	// trader ends up transferring against the wrong document.
	if v := cms.String(slip, "payment_slip_url"); v != "" {
		out["payment_slip_url"] = v
	}
	if v := number64(details, "total_payable_lkr"); v != 0 {
		out["payable_lkr"] = v
	}
	if v := number64(details, "exchange_rate"); v != 0 {
		out["exchange_rate"] = v
	}
	return true, out
}

// lineItems is the invoice broken down the way it was priced: one entry per
// container, since that is the unit SLPA charges for and the one a trader
// checks a total against.
func lineItems(details map[string]any) []map[string]any {
	raw, _ := details["items"].([]any)
	items := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, map[string]any{
			"container_no":   cms.String(item, "container_no"),
			"container_size": cms.String(item, "container_size"),
			"container_type": cms.String(item, "container_type"),
			"service_name":   cms.String(item, "service_name"),
			"quantity":       cms.String(item, "quantity"),
			"total_lkr":      number64(item, "total_lkr"),
		})
	}
	return items
}

// paymentSlip is what the bank is handed: who is paying, against which
// reference, for how much. Recorded so the panel can state it rather than
// sending the trader into the PDF to find out what they are transferring.
func paymentSlip(slip map[string]any) map[string]any {
	return map[string]any{
		"number":      cms.String(slip, "number"),
		"number_type": cms.String(slip, "number_type"),
		"shipper":     cms.String(slip, "shipper"),
		"consignee":   cms.String(slip, "consignee"),
		"total":       cms.String(slip, "total"),
		"date":        cms.String(slip, "date"),
		"time":        cms.String(slip, "time"),
	}
}

// mapAt reads a nested object. A missing one is an empty map, so every read
// through it answers "" or zero rather than panicking on an answer shaped
// differently than expected.
func mapAt(m map[string]any, key string) map[string]any {
	nested, _ := m[key].(map[string]any)
	if nested == nil {
		return map[string]any{}
	}
	return nested
}

// number64 reads an amount. JSON numbers arrive as float64; one sent as a
// string is read too, since the CMS has been seen to send both.
func number64(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if f, err := parseFloat(v); err == nil {
			return f
		}
	}
	return 0
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
