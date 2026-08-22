package ecdn

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpreter_BuildRequest(t *testing.T) {
	req := NewInterpreter().BuildRequest(map[string]any{"payload": fullForm()})

	body, err := json.Marshal(req)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))

	// The declaration travels as a string in xml_payload, and identity is not in
	// the body — it is the bearer token and the slpacmsuser-key header, both
	// applied by remote.Manager from the service's configuration.
	doc, ok := out["xml_payload"].(string)
	require.True(t, ok, "xml_payload must be a string")
	assert.Contains(t, doc, "<CusDecNote>")
	assert.Contains(t, doc, "<ContainerMark>MSCU-849201-9</ContainerMark>")
	assert.Len(t, out, 1, "the upload body carries only the declaration")
}

// The contract cannot fail a call, so a declaration that will not assemble is
// sent empty and the CMS's own validation answers it. What must not happen is a
// half-built document reaching SLPA.
func TestInterpreter_BuildRequestSendsNothingItCannotAssemble(t *testing.T) {
	i := NewInterpreter()

	t.Run("no form in the inputs", func(t *testing.T) {
		assert.Equal(t, UploadRequest{}, i.BuildRequest(map[string]any{}))
	})

	t.Run("a form with no containers", func(t *testing.T) {
		form := fullForm()
		delete(form, "containers")
		assert.Equal(t, UploadRequest{}, i.BuildRequest(map[string]any{"payload": form}))
	})
}

func TestInterpreter_InterpretAccepted(t *testing.T) {
	accepted, out := NewInterpreter().Interpret(nil, map[string]any{
		"status": "SUCCESS", "message": "ECDN validated", "reference": "ECDN-9001",
	})
	assert.True(t, accepted)
	assert.Equal(t, "ECDN-9001", out["reference"])
	assert.NotContains(t, out, "error")
}

// The response SLPA's CMS actually returns: the verdict is nested under "data",
// and the top-level "status" is an envelope code (1 for a served request) that
// says nothing about the declaration.
func TestInterpreter_InterpretAcceptedInsideTheDataEnvelope(t *testing.T) {
	accepted, out := NewInterpreter().Interpret(nil, map[string]any{
		"data": map[string]any{
			"status":        "ACCEPTED",
			"validated_at":  "2026-08-22T18:13:57+05:30",
			"cusdec_serial": "BIBE1CBEX1-2026-E-10512026",
		},
		"status":  float64(1),
		"openapi": "3.0.3",
	})
	assert.True(t, accepted)
	// The serial the CMS filed the declaration under is worth recording: it is
	// how SLPA refers to it afterwards.
	assert.Equal(t, "BIBE1CBEX1-2026-E-10512026", out["cusdec_serial"])
	assert.Equal(t, "2026-08-22T18:13:57+05:30", out["validated_at"])
	// The nested verdict wins over the envelope's numeric status.
	assert.Equal(t, "ACCEPTED", out["status"])
	assert.NotContains(t, out, "error")
}

func TestInterpreter_InterpretRejections(t *testing.T) {
	i := NewInterpreter()

	t.Run("field-keyed errors are rendered in a stable order", func(t *testing.T) {
		resp := map[string]any{"status": "FAILED", "errors": map[string]any{
			"Cusdec_No": "Duplicate declaration", "Terminal": "Unknown terminal code",
		}}
		accepted, out := i.Interpret(nil, resp)
		assert.False(t, accepted)

		msg := out["error"].(string)
		assert.Contains(t, msg, "Duplicate declaration")
		assert.Contains(t, msg, "_(Terminal)_")
		// Sorted, so the same rejection always reads the same way.
		assert.Less(t, indexOf(msg, "Cusdec_No"), indexOf(msg, "Terminal"))
	})

	t.Run("an error list is rendered", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
			"errors": []any{map[string]any{"message": "Schema validation failed", "field": "Volume_CBM"}},
		})
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "Schema validation failed")
		assert.Contains(t, out["error"], "_(Volume_CBM)_")
	})

	// A 200 whose body does not confirm success must not be read as accepted:
	// recording a Service Order as raisable when the CMS never stored the
	// declaration is the worse failure.
	t.Run("an unrecognized status is not an acceptance", func(t *testing.T) {
		accepted, _ := i.Interpret(nil, map[string]any{"status": "PENDING_REVIEW"})
		assert.False(t, accepted)
	})

	// The shape the CMS uses to refuse a request it never read: one object under
	// "error", with the reason and a support code.
	t.Run("an object-shaped refusal is rendered with its code", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
			"error": map[string]any{
				"code":    "MISSING_CLIENT_HEADER",
				"message": "Client identifier header 'slpacmsuser-key' is required.",
			},
			"status":  float64(0),
			"openapi": "3.0.3",
		})
		assert.False(t, accepted)

		msg := out["error"].(string)
		assert.Contains(t, msg, "Client identifier header 'slpacmsuser-key' is required.")
		assert.Contains(t, msg, "_(MISSING_CLIENT_HEADER)_")
		// The reason must not be swallowed, leaving only the wrapper prose.
		assert.NotEqual(t, "SLPA did not accept your cargo declaration:\n\nPlease correct the details and submit again.", msg)
	})

	t.Run("an object-shaped refusal with no code", func(t *testing.T) {
		_, out := i.Interpret(nil, map[string]any{
			"error": map[string]any{"message": "Duplicate declaration."},
		})
		assert.Contains(t, out["error"], "- Duplicate declaration.")
	})

	t.Run("a rejection nested in the data envelope is read", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
			"data": map[string]any{"status": "REJECTED", "errors": map[string]any{
				"Cusdec_No": "Duplicate declaration",
			}},
			"status": float64(1),
		})
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "Duplicate declaration")
	})

	// The envelope's numeric status is not a verdict, so a body with no nested
	// outcome is a rejection rather than an acceptance.
	t.Run("an envelope status alone is not an acceptance", func(t *testing.T) {
		accepted, _ := i.Interpret(nil, map[string]any{"status": float64(1), "openapi": "3.0.3"})
		assert.False(t, accepted)
	})

	t.Run("a success flag without a status is honoured", func(t *testing.T) {
		accepted, _ := i.Interpret(nil, map[string]any{"success": true})
		assert.True(t, accepted)
	})

	t.Run("transport failure", func(t *testing.T) {
		accepted, out := i.Interpret(errors.New("dial tcp: timeout"), nil)
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "could not reach the SLPA")
	})

}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
