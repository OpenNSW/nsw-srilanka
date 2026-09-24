package cusdec

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/OpenNSW/core/remote"
)

// eventVerified is the discriminator SLC Edge puts on a verify response.
const eventVerified = "CUSDEC_VERIFIED"

// VerifyInterpreter adapts the generic API-call plugin to the SLC Edge
// declaration verify endpoint (POST /api/declaration/v1/verify).
//
// Verify is the submission's dry run: the same Annex A payload, checked and
// priced without registering anything. It answers with the duty ASYCUDA would
// assess, or with the same segment-keyed errors a rejected submission returns.
//
// Two things separate it from the submission interpreter:
//
//   - the body is raw JSON, not multipart. This deliberately does not
//     implement BuildParts, which is what keeps the plugin on the JSON path;
//     attachments are not verified and the endpoint does not accept them.
//   - nothing here advances the declaration. The call records what came back
//     and the workflow returns to the form, so a trader can verify as often as
//     they like before submitting.
type VerifyInterpreter struct{}

// NewVerifyInterpreter returns the verify interpreter.
func NewVerifyInterpreter() VerifyInterpreter { return VerifyInterpreter{} }

// BuildRequest maps the trader form onto the same Annex A payload the
// submission sends. A form that cannot be mapped is sent as-is so the endpoint
// answers with its own validation errors, which name fields far better than a
// refusal assembled here could.
func (VerifyInterpreter) BuildRequest(inputs map[string]any) remote.Body {
	payload, _, err := buildFromInputs(inputs)
	if err != nil {
		return remote.JSONBody{V: inputs}
	}
	return remote.JSONBody{V: payload}
}

// Interpret turns the verify response into the summary the trader reads.
//
// accepted reports whether the declaration verified, which is the answer to
// the trader's question rather than a statement about the call: a declaration
// that failed validation was still verified successfully, and the summary says
// why it failed. A call that never completed is the only case with no summary
// of its own to give.
func (VerifyInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	if callErr != nil {
		return false, map[string]any{
			"summary": "### Verification unavailable\n\nSri Lanka Customs could not be reached. " +
				"Nothing has been submitted — try again, or submit the declaration to have it checked on arrival.",
		}
	}

	var r verifyResponse
	if raw, err := json.Marshal(resp); err == nil {
		_ = json.Unmarshal(raw, &r)
	}

	if r.Payload.Verified {
		return true, map[string]any{
			"verified": true,
			"summary":  renderDuties(r.Payload.Duties),
			"total":    r.Payload.Duties.total(),
		}
	}

	return false, map[string]any{
		"verified": false,
		"summary":  renderVerifyErrors(r.Payload.Errors),
	}
}

// verifyResponse is the verify endpoint's answer. The envelope matches the
// notification envelope (§4.6) even though this arrives on the response to the
// call rather than as a pushed event.
type verifyResponse struct {
	EventType   string `json:"eventType"`
	ProcessedAt string `json:"processedAt"`
	Payload     struct {
		Verified bool            `json:"verified"`
		Duties   verifyDuties    `json:"duties"`
		Errors   json.RawMessage `json:"errors"`
	} `json:"payload"`
}

// verifyDuties is the assessment, split the way ASYCUDA computes it: charges
// that fall on the declaration as a whole, and charges that fall on one item.
type verifyDuties struct {
	GlobalDuties   []dutyLine `json:"globalDuties"`
	ItemDutiesList []struct {
		ItemSequenceNumeric int        `json:"itemSequenceNumeric"`
		DutyTaxFees         []dutyLine `json:"dutyTaxFees"`
	} `json:"itemDutiesList"`
}

type dutyLine struct {
	TypeCode          string  `json:"typeCode"`
	TaxBaseAmount     float64 `json:"taxBaseAmount"`
	TaxRateNumeric    float64 `json:"taxRateNumeric"`
	TaxAssessedAmount float64 `json:"taxAssessedAmount"`
	PaymentMethodCode string  `json:"paymentMethodCode"`
}

// total is every assessed charge, global and per item. It is what the trader
// would pay, and the figure the payment step asks for after a real submission.
func (d verifyDuties) total() float64 {
	var t float64
	for _, l := range d.GlobalDuties {
		t += l.TaxAssessedAmount
	}
	for _, item := range d.ItemDutiesList {
		for _, l := range item.DutyTaxFees {
			t += l.TaxAssessedAmount
		}
	}
	return t
}

// renderDuties writes the assessment as markdown. A verified declaration with
// no charges at all is still a useful answer, so it says so rather than
// rendering an empty table.
func renderDuties(d verifyDuties) string {
	var b strings.Builder
	b.WriteString("### Verified\n\nSri Lanka Customs accepted this declaration for checking. " +
		"Nothing has been submitted yet.\n")

	if len(d.GlobalDuties) == 0 && len(d.ItemDutiesList) == 0 {
		b.WriteString("\nNo duty was assessed against it.\n")
		return b.String()
	}

	if len(d.GlobalDuties) > 0 {
		b.WriteString("\n**Declaration charges**\n\n")
		writeDutyLines(&b, d.GlobalDuties)
	}

	// Sorted so the same assessment always reads the same way, whatever order
	// the items came back in.
	items := d.ItemDutiesList
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ItemSequenceNumeric < items[j].ItemSequenceNumeric
	})
	for _, item := range items {
		if len(item.DutyTaxFees) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n**Item %d**\n\n", item.ItemSequenceNumeric)
		writeDutyLines(&b, item.DutyTaxFees)
	}

	fmt.Fprintf(&b, "\n**Total assessed: %s**\n", money(d.total()))
	return b.String()
}

// writeDutyLines writes the charges as a list rather than a table. The panel
// renders CommonMark without the GFM extension, so a pipe table arrives as a
// row of literal pipes on one line; a list is read the same by both.
func writeDutyLines(b *strings.Builder, lines []dutyLine) {
	for _, l := range lines {
		fmt.Fprintf(b, "- **%s** — %s assessed (base %s, rate %s)\n",
			l.TypeCode, money(l.TaxAssessedAmount), money(l.TaxBaseAmount), rate(l.TaxRateNumeric))
	}
}

// renderVerifyErrors reports a declaration that did not verify, reusing the
// submission's reading of the segment-keyed errors object (§4.5) so the same
// fault is worded the same way whichever endpoint reported it.
func renderVerifyErrors(raw json.RawMessage) string {
	return "### Not verified\n\nSri Lanka Customs would reject this declaration as it stands. " +
		"Nothing has been submitted.\n\n" + describeErrors(raw) + "\n"
}

// money renders an amount without the trailing zeroes a float carries, so a
// whole-number charge reads as 1100 rather than 1100.00.
func money(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// rate renders a percentage the way the assessment states it, keeping a rate
// of 250 distinct from one of 2.
func rate(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
