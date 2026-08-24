package cms

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The envelope every CMS endpoint uses: the outcome under "data", an envelope
// code on top that says only that the request was served.
func TestFlattenAndAccepted(t *testing.T) {
	body := Flatten(map[string]any{
		"data":    map[string]any{"status": "ACCEPTED", "cusdec_serial": "BIBE1CBEX1-2026-E-10512026"},
		"status":  float64(1),
		"openapi": "3.0.3",
	})

	assert.Equal(t, "ACCEPTED", body["status"], "the nested verdict wins over the envelope code")
	assert.Equal(t, "BIBE1CBEX1-2026-E-10512026", body["cusdec_serial"])
	assert.True(t, Accepted(body))
	assert.False(t, HasErrors(body))
}

func TestAccepted_UnrecognizedIsRefused(t *testing.T) {
	// An envelope code is not a verdict, and an unknown status is not an
	// acceptance: treating an unstored submission as stored is the worse failure.
	for name, body := range map[string]map[string]any{
		"envelope code only": {"status": float64(1), "openapi": "3.0.3"},
		"unknown status":     {"status": "PENDING_REVIEW"},
		"nothing at all":     {},
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, Accepted(body))
		})
	}

	t.Run("a success flag without a status is honoured", func(t *testing.T) {
		assert.True(t, Accepted(map[string]any{"success": true}))
		assert.False(t, Accepted(map[string]any{"success": false}))
	})
}

// The four refusal shapes the CMS uses, all of which must produce a reason: a
// blank rejection tells the trader nothing they can act on.
func TestReasons(t *testing.T) {
	t.Run("a validation failure renders its per-field details", func(t *testing.T) {
		body := Flatten(map[string]any{
			"error": map[string]any{
				"code":    "VALIDATION_FAILED",
				"message": "The given request data was invalid.",
				"details": map[string]any{
					"containers.0.cbm":       []any{"The containers.0.cbm must be at least 0.01."},
					"containers.0.commodity": []any{"The selected containers.0.commodity is invalid."},
				},
			},
			"status": float64(0),
		})
		require.True(t, HasErrors(body))

		// Sorted by field, so the same refusal reads the same way twice.
		assert.Equal(t, []string{
			"- The containers.0.cbm must be at least 0.01. _(containers.0.cbm)_",
			"- The selected containers.0.commodity is invalid. _(containers.0.commodity)_",
		}, Reasons(body))
	})

	t.Run("several messages for one field", func(t *testing.T) {
		reasons := Reasons(map[string]any{"error": map[string]any{"details": map[string]any{
			"cusdec_no": []any{"The cusdec_no is required.", "The cusdec_no must be a string."},
		}}})
		assert.Len(t, reasons, 2)
	})

	t.Run("a detail sent as a lone string", func(t *testing.T) {
		assert.Equal(t, []string{"- Unknown service. _(service_id)_"},
			Reasons(map[string]any{"error": map[string]any{"details": map[string]any{
				"service_id": "Unknown service.",
			}}}))
	})

	// A refusal with no details still has a message and a code worth showing.
	t.Run("an object with no details", func(t *testing.T) {
		assert.Equal(t, []string{"- Client identifier header 'slpacmsuser-key' is required. _(MISSING_CLIENT_HEADER)_"},
			Reasons(map[string]any{"error": map[string]any{
				"code":    "MISSING_CLIENT_HEADER",
				"message": "Client identifier header 'slpacmsuser-key' is required.",
			}}))
	})

	t.Run("a field-keyed errors map", func(t *testing.T) {
		assert.Equal(t, []string{
			"- Duplicate declaration. _(Cusdec_No)_",
			"- Unknown terminal code. _(Terminal)_",
		}, Reasons(map[string]any{"errors": map[string]any{
			"Terminal":  "Unknown terminal code.",
			"Cusdec_No": "Duplicate declaration.",
		}}))
	})

	t.Run("an errors list", func(t *testing.T) {
		assert.Equal(t, []string{"- Schema validation failed. _(Volume_CBM)_", "- Plain string reason."},
			Reasons(map[string]any{"errors": []any{
				map[string]any{"message": "Schema validation failed.", "field": "Volume_CBM"},
				"Plain string reason.",
			}}))
	})

	t.Run("a string-valued error", func(t *testing.T) {
		assert.Equal(t, []string{"- Something went wrong."},
			Reasons(map[string]any{"error": "Something went wrong."}))
	})

	t.Run("nothing to say", func(t *testing.T) {
		assert.Empty(t, Reasons(map[string]any{"status": "FAILED"}))
	})
}

func TestHasErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		body map[string]any
		want bool
	}{
		"empty error object": {map[string]any{"error": map[string]any{}}, false},
		"populated object":   {map[string]any{"error": map[string]any{"code": "X"}}, true},
		"empty errors list":  {map[string]any{"errors": []any{}}, false},
		"errors list":        {map[string]any{"errors": []any{"x"}}, true},
		"string error":       {map[string]any{"error": "x"}, true},
		"blank string error": {map[string]any{"error": "   "}, false},
		"no error fields":    {map[string]any{"status": "SUCCESS"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasErrors(tc.body))
		})
	}
}
