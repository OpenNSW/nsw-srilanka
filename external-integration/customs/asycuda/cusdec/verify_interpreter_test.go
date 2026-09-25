package cusdec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decoded turns a response document into the shape the remote client hands the
// interpreter.
func decoded(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

// The sample response Sri Lanka Customs supplied with the verify endpoint.
const sampleVerified = `{
  "eventType": "CUSDEC_VERIFIED",
  "payload": {
    "duties": {
      "globalDuties": [
        {"paymentMethodCode":"1","taxAssessedAmount":1100,"taxBaseAmount":550,"taxRateNumeric":2.0,"typeCode":"EPF"},
        {"paymentMethodCode":"1","taxAssessedAmount":250,"taxBaseAmount":1,"taxRateNumeric":250.0,"typeCode":"COM"}
      ],
      "itemDutiesList": [
        {"dutyTaxFees":[{"paymentMethodCode":"1","taxAssessedAmount":0,"taxBaseAmount":125,"taxRateNumeric":0.0,"typeCode":"CED"}],
         "itemSequenceNumeric":1}
      ]
    },
    "errors": {},
    "verified": true
  },
  "processedAt": "2026-09-23T10:26:37Z"
}`

func TestVerifyInterpreter_ReadsTheAssessment(t *testing.T) {
	accepted, out := VerifyInterpreter{}.Interpret(nil, decoded(t, sampleVerified))

	assert.True(t, accepted)
	assert.Equal(t, true, out["verified"])
	// 1100 + 250 global, plus 0 on item 1.
	assert.Equal(t, float64(1350), out["total"])

	summary, _ := out["summary"].(string)
	for _, want := range []string{
		"### Verified",
		"**Declaration charges**",
		"- **EPF** — 1100 assessed (base 550, rate 2)",
		"- **COM** — 250 assessed (base 1, rate 250)",
		"**Item 1**",
		"- **CED** — 0 assessed (base 125, rate 0)",
		"**Total assessed: 1350**",
	} {
		assert.Contains(t, summary, want)
	}
	assert.Contains(t, summary, "Nothing has been submitted",
		"verifying must not read as having filed the declaration")
	assert.NotContains(t, summary, "|---",
		"the panel renders CommonMark without GFM, so a pipe table arrives as literal text")
}

// The endpoint answers a failed verification with the submission's own
// segment-keyed errors object, so it is read the same way.
func TestVerifyInterpreter_ReportsWhyItWouldBeRejected(t *testing.T) {
	accepted, out := VerifyInterpreter{}.Interpret(nil, decoded(t, `{
      "eventType": "CUSDEC_VERIFIED",
      "payload": {
        "verified": false,
        "errors": {
          "0": [{"code":403,"description":"Invalid importer code 900000000001"}],
          "1": [{"code":449,"description":"Marks and Number 1 is mandatory"}]
        }
      }
    }`))

	assert.False(t, accepted)
	assert.Equal(t, false, out["verified"])

	summary, _ := out["summary"].(string)
	assert.Contains(t, summary, "### Not verified")
	assert.Contains(t, summary, "Invalid importer code 900000000001")
	assert.Contains(t, summary, "Marks and Number 1 is mandatory")
	assert.Contains(t, summary, "Nothing has been submitted")
	assert.NotContains(t, summary, `{"0":`, "the raw errors object must not reach the trader")
}

// A declaration can verify with nothing to pay. An empty table would read as a
// missing answer rather than as the answer.
func TestVerifyInterpreter_SaysSoWhenNoDutyIsAssessed(t *testing.T) {
	accepted, out := VerifyInterpreter{}.Interpret(nil, decoded(t, `{
      "payload": {"verified": true, "duties": {"globalDuties": [], "itemDutiesList": []}, "errors": {}}
    }`))

	assert.True(t, accepted)
	assert.Equal(t, float64(0), out["total"])
	assert.Contains(t, out["summary"], "No duty was assessed")
}

// Verifying is not filing, so a call that never landed must not look like one
// that came back clean.
func TestVerifyInterpreter_AnUnreachableServiceIsNotAVerification(t *testing.T) {
	accepted, out := VerifyInterpreter{}.Interpret(errors.New("dial tcp: connection refused"), nil)

	assert.False(t, accepted)
	assert.NotContains(t, out, "verified")
	assert.Contains(t, out["summary"], "Verification unavailable")
	assert.Contains(t, out["summary"], "Nothing has been submitted")
}

// Item charges are rendered in item order however the response listed them, so
// one assessment always reads the same way.
func TestVerifyInterpreter_OrdersItemsBySequence(t *testing.T) {
	_, out := VerifyInterpreter{}.Interpret(nil, decoded(t, `{
      "payload": {"verified": true, "duties": {"itemDutiesList": [
        {"itemSequenceNumeric": 2, "dutyTaxFees": [{"typeCode":"VAT","taxAssessedAmount":10}]},
        {"itemSequenceNumeric": 1, "dutyTaxFees": [{"typeCode":"CED","taxAssessedAmount":5}]}
      ]}}
    }`))

	summary, _ := out["summary"].(string)
	assert.Less(t, strings.Index(summary, "**Item 1**"), strings.Index(summary, "**Item 2**"))
	assert.Equal(t, float64(15), out["total"])
}

// The body is the same Annex A payload the submission sends, as raw JSON --
// the endpoint does not accept multipart, and attachments are not verified.
func TestVerifyInterpreter_SendsTheDeclarationAsJSON(t *testing.T) {
	body := VerifyInterpreter{}.BuildRequest(map[string]any{"payload": sampleForm()})

	jsonBody, ok := body.(remote.JSONBody)
	require.True(t, ok, "verify must not go out as multipart")

	encoded, err := json.Marshal(jsonBody.V)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"baseGeneralSegment"`)
	assert.Contains(t, string(encoded), `"goodsShipments"`)

	// Implementing BuildParts is what routes an interpreter to multipart, so
	// the signature here has to be the one the plugin switches on --
	// plugins.MultipartInterpreter's, down to the context.Context. Go matches
	// method sets exactly: written with any in place of context.Context this
	// assertion can never hold, and would pass just as readily on an
	// interpreter that had gained the method.
	//
	// The interface is restated rather than imported because
	// internal/tasks/plugins imports this package, so naming it here would be
	// a cycle.
	_, isMultipart := any(VerifyInterpreter{}).(interface {
		BuildParts(context.Context, map[string]any) ([]remote.Part, error)
	})
	assert.False(t, isMultipart, "verify must not go out as multipart")
}
