package cdn

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCDNInterpreter_BuildRequest(t *testing.T) {
	sub := NewCDNInterpreter().BuildRequest(map[string]any{"payload": fullForm()})

	body, err := json.Marshal(sub.(remote.JSONBody).V)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"containerNumber":"MSCU8492019"`)
	assert.Contains(t, string(body), `"cusDecRefs":[{"year":"2026","office":"CBEX1","serial":"E","number":1047}]`)
}

// A form that cannot be mapped must surface the reason to the trader rather
// than putting a half-built note in front of Customs.
func TestCDNInterpreter_BuildRequestOnUnusableForm(t *testing.T) {
	req := NewCDNInterpreter().BuildRequest(map[string]any{})

	m, ok := req.(remote.JSONBody).V.(map[string]any)
	require.True(t, ok, "expected an error object, got %T", req)
	assert.Contains(t, m["error"], "could not be read")
}

func TestCDNInterpreter_InterpretAccepted(t *testing.T) {
	resp := map[string]any{
		"edgeId":     "5516e4c8-a93d-429d-8a18-6a484d331176",
		"status":     "RECEIVED",
		"receivedAt": "2026-06-26T04:04:52Z",
		"message":    "CDN submission accepted and queued for processing.",
	}

	accepted, out := NewCDNInterpreter().Interpret(nil, resp)
	assert.True(t, accepted)
	// The edgeId is what the §7.2 callback is correlated by, so it has to be
	// captured on every accepted submission.
	assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", out["edgeId"])
	assert.Equal(t, "RECEIVED", out["status"])
	assert.NotContains(t, out, "error")
}

func TestCDNInterpreter_InterpretRejections(t *testing.T) {
	interp := NewCDNInterpreter()

	t.Run("spec segment-keyed errors object", func(t *testing.T) {
		var resp map[string]any
		require.NoError(t, json.Unmarshal([]byte(`{
		  "status": "REJECTED",
		  "errors": {
		    "0": [{"code": 331, "description": "Missing office Code"}],
		    "1": [{"code": 410, "description": "Missing container number", "fieldRef": "containerNumber"}]
		  }
		}`), &resp))

		accepted, out := interp.Interpret(nil, resp)
		assert.False(t, accepted)
		msg := out["error"].(string)
		assert.Contains(t, msg, "Missing office Code")
		assert.Contains(t, msg, "Item 1: Missing container number")
		assert.Contains(t, msg, "_(containerNumber)_")
	})

	t.Run("flat error string", func(t *testing.T) {
		accepted, out := interp.Interpret(nil, map[string]any{"error": "cusDecRefs is required"})
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "cusDecRefs is required")
	})

	t.Run("transport failure with empty body", func(t *testing.T) {
		accepted, out := interp.Interpret(errors.New("dial tcp: timeout"), nil)
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "could not reach Sri Lanka Customs")
	})

	// A local assembly failure never reached Customs, so it must not be
	// reported to the trader as their rejection.
	t.Run("local build failure", func(t *testing.T) {
		accepted, out := interp.Interpret(&buildError{"This dispatch note is not linked to a registered customs declaration."}, nil)
		assert.False(t, accepted)
		msg := out["error"].(string)
		assert.Contains(t, msg, "could not be submitted")
		assert.Contains(t, msg, "not linked to a registered customs declaration")
		assert.NotContains(t, msg, "was not accepted by Sri Lanka Customs")
	})

	// A 202-shaped body carrying a non-empty errors array is a rejection, not
	// an acceptance with noise attached.
	t.Run("accepted status with errors present", func(t *testing.T) {
		accepted, _ := interp.Interpret(nil, map[string]any{
			"status": "RECEIVED",
			"errors": []any{map[string]any{"message": "duplicate container"}},
		})
		assert.False(t, accepted)
	})

	t.Run("unknown status", func(t *testing.T) {
		accepted, _ := interp.Interpret(nil, map[string]any{"status": "PENDING_SOMETHING"})
		assert.False(t, accepted)
	})
}
