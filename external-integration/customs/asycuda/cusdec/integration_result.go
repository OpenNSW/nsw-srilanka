package cusdec

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// amountToPay is what the trader is asked to settle on the payment step: what
// is still due on the declaration, not what was assessed.
//
// A figure ASYCUDA states wins over one this side reconstructs, because it is
// the assessment while a sum of lines is only an estimate of it -- the v1.7
// example had the two disagree, 1254 against lines totalling 1244. In order:
//
//  1. amountPayable (v1.9): what is still due. 0 is an answer: a declaration
//     settled from a prepayment account owes nothing, and no Payment
//     Notification follows for it.
//  2. amountToPay (v1.7), for a sender still on that shape.
//  3. totalAssessedAmount less amountPaid (v1.9), the rule amountPayable
//     follows, for a result that states the parts but not the difference.
//  4. The v1.9 duties breakdown, summed.
//  5. The v1.7 tax lines, summed -- every result sent against v1.6.
func amountToPay(p cusdecResultPayload) float64 {
	switch {
	case p.AmountPayable != nil:
		return *p.AmountPayable
	case p.AmountToPay != nil:
		return *p.AmountToPay
	case p.TotalAssessedAmount != nil:
		paid := 0.0
		if p.AmountPaid != nil {
			paid = *p.AmountPaid
		}
		return math.Round((*p.TotalAssessedAmount-paid)*100) / 100
	case len(p.Duties.GlobalDuties) > 0 || len(p.Duties.ItemDutiesList) > 0:
		return p.Duties.total()
	default:
		return totalTaxes(p.Taxes)
	}
}

// totalTaxes sums the assessed tax lines from a §6.2 integration result. The
// spec carries the duty as a per-code breakdown, while the payment step that
// follows asks the trader for a single figure.
func totalTaxes(taxes []TaxEntry) float64 {
	var total float64
	for _, t := range taxes {
		total += t.Amount
	}
	return total
}

// describeErrors renders the §4.5 segment-keyed errors object as a readable
// message for the trader.
//
// The raw JSON was previously passed through verbatim, which put
// `{"0":[{"code":300,...}]}` in front of a trader who has no way to read it.
// Segment "0" is the declaration as a whole; any other key is an item number.
// Anything that does not parse falls back to the raw text rather than being
// dropped — an unreadable reason still beats no reason.
func describeErrors(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "Sri Lanka Customs rejected the declaration without giving a reason."
	}

	var segments map[string][]struct {
		Code        any    `json:"code"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &segments); err != nil || len(segments) == 0 {
		return strings.TrimSpace(string(raw))
	}

	// Sort so the general segment leads and items follow in order, rather than
	// in Go's randomized map order.
	keys := make([]string, 0, len(segments))
	for k := range segments {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, erri := strconv.Atoi(keys[i])
		nj, errj := strconv.Atoi(keys[j])
		if erri == nil && errj == nil {
			return ni < nj
		}
		return keys[i] < keys[j]
	})

	var b strings.Builder
	for _, key := range keys {
		entries := segments[key]
		if len(entries) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if key == "0" {
			b.WriteString("**Declaration**\n")
		} else {
			b.WriteString("**Item " + key + "**\n")
		}
		for _, e := range entries {
			msg := strings.TrimSpace(e.Description)
			if msg == "" {
				msg = "Rejected without a description."
			}
			if code := formatCode(e.Code); code != "" {
				msg = fmt.Sprintf("%s _(code %s)_", msg, code)
			}
			b.WriteString("- " + msg + "\n")
		}
	}

	if b.Len() == 0 {
		return strings.TrimSpace(string(raw))
	}
	return strings.TrimSpace(b.String())
}

// formatCode renders an error code, which arrives as a JSON number and so
// decodes to float64, without the trailing ".0" a plain format would add.
func formatCode(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case string:
		return strings.TrimSpace(n)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", n)
	}
}
