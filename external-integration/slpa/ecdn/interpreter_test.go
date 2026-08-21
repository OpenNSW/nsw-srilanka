package ecdn

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpreter_BuildCall(t *testing.T) {
	i := NewInterpreter()

	req, headers, err := i.BuildCall(context.Background(), "consignment-1", map[string]any{
		"payload":      fullForm(),
		ClientKeyInput: "UkLWZg9DAJ",
	})
	require.NoError(t, err)

	// SLPA identifies the submitting company by this header, not by the body.
	assert.Equal(t, "UkLWZg9DAJ", headers[ClientKeyHeader])

	body, err := json.Marshal(req)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))

	// The declaration travels as a string in xml_payload, and identity is not in
	// the body — it is the bearer token and the slpacmsuser-key header.
	doc, ok := out["xml_payload"].(string)
	require.True(t, ok, "xml_payload must be a string")
	assert.Contains(t, doc, "<CusDecNote>")
	assert.Contains(t, doc, "<ContainerMark>MSCU-849201-9</ContainerMark>")
	assert.Len(t, out, 1, "the upload body carries only the declaration")
}

func TestInterpreter_RefusesToSendWithoutAnIdentifiedCompany(t *testing.T) {
	i := NewInterpreter()

	t.Run("unreadable form", func(t *testing.T) {
		_, _, err := i.BuildCall(context.Background(), "c1", map[string]any{ClientKeyInput: "K"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not be read")
	})

	// Reaching SLPA without a client key would file the declaration against no
	// company at all, so it is refused before anything is sent.
	t.Run("no client key mapped in", func(t *testing.T) {
		_, _, err := i.BuildCall(context.Background(), "c1", map[string]any{"payload": fullForm()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered with the SLPA")
	})

	t.Run("blank client key", func(t *testing.T) {
		_, _, err := i.BuildCall(context.Background(), "c1", map[string]any{
			"payload": fullForm(), ClientKeyInput: "   ",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered with the SLPA")
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

	t.Run("a success flag without a status is honoured", func(t *testing.T) {
		accepted, _ := i.Interpret(nil, map[string]any{"success": true})
		assert.True(t, accepted)
	})

	t.Run("transport failure", func(t *testing.T) {
		accepted, out := i.Interpret(errors.New("dial tcp: timeout"), nil)
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "could not reach the SLPA")
	})

	// A declaration that never left NSW must not be reported as SLPA's rejection.
	t.Run("local build failure", func(t *testing.T) {
		accepted, out := i.Interpret(&buildError{"Add at least one container before submitting."}, nil)
		assert.False(t, accepted)
		msg := out["error"].(string)
		assert.Contains(t, msg, "could not be submitted")
		assert.NotContains(t, msg, "did not accept")
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
