package cms

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// body decodes a response exactly as the CMS sent it, so the fixtures below can
// be its own payloads verbatim.
func body(t *testing.T, response string) map[string]any {
	t.Helper()

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(response), &resp))
	return Flatten(resp)
}

// An accepted declaration: the outcome nested under "data", the envelope's own
// status on top saying only that the request was served.
func TestAccepted(t *testing.T) {
	b := body(t, `{
		"data": {
			"status": "ACCEPTED",
			"validated_at": "2026-08-22T18:13:57+05:30",
			"cusdec_serial": "BIBE1CBEX1-2026-E-10512026"
		},
		"status": 1,
		"openapi": "3.0.3"
	}`)

	assert.Equal(t, "ACCEPTED", b["status"], "the nested outcome wins over the envelope's status")
	assert.Equal(t, "BIBE1CBEX1-2026-E-10512026", b["cusdec_serial"])
	assert.True(t, Accepted(b))
	assert.False(t, HasErrors(b))
	assert.Empty(t, Reasons(b))
}

// A call that never reached the service leaves no body behind, and every reader
// here has to cope with that rather than the caller checking first.
func TestFlatten_NoBody(t *testing.T) {
	b := Flatten(nil)

	assert.NotNil(t, b, "a reader is handed a map it can read from, never nil")
	assert.Empty(t, b)
	assert.False(t, Accepted(b))
	assert.False(t, HasErrors(b))
	assert.Empty(t, Reasons(b))
}

// The envelope is copied, so what a caller does with the flattened body cannot
// change the response it decoded — and the container key is not part of the
// outcome.
func TestFlatten_CopiesTheEnvelope(t *testing.T) {
	resp := map[string]any{
		"data":    map[string]any{"status": "ACCEPTED"},
		"status":  float64(1),
		"openapi": "3.0.3",
	}

	b := Flatten(resp)
	b["status"] = "TAMPERED"
	delete(b, "openapi")

	assert.Equal(t, float64(1), resp["status"], "the caller's envelope is untouched")
	assert.Equal(t, "3.0.3", resp["openapi"])
	assert.Equal(t, map[string]any{"status": "ACCEPTED"}, resp["data"])
	assert.NotContains(t, Flatten(resp), "data", "the container itself is not part of the outcome")
}

// The envelope's status is not a verdict, and an unrecognized one is not an
// acceptance: recording an unstored submission as stored is the worse failure.
func TestAccepted_UnrecognizedIsRefused(t *testing.T) {
	for name, response := range map[string]string{
		"envelope status only":  `{"status": 1, "openapi": "3.0.3"}`,
		"an unknown outcome":    `{"data": {"status": "PENDING_REVIEW"}, "status": 1}`,
		"an order's own state":  `{"data": {"status": "client_new_actclk"}, "status": 1}`,
		"a body with no status": `{"data": {}, "status": 1}`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, Accepted(body(t, response)))
		})
	}
}

// A refusal the CMS states in one message: a declaration it already holds, and a
// call it cannot attribute to a company. Both must reach the trader with the code
// SLPA support asks for.
func TestReasons_RefusedWithAMessage(t *testing.T) {
	t.Run("duplicate declaration", func(t *testing.T) {
		b := body(t, `{
			"error": {
				"code": "DUPLICATE_CUSDEC",
				"message": "CUSDEC serial number 'BIBE1CBEX1-2026-E-10502026' has already been submitted."
			},
			"status": 0,
			"openapi": "3.0.3"
		}`)

		require.True(t, HasErrors(b))
		assert.False(t, Accepted(b))
		assert.Equal(t, []string{
			"- CUSDEC serial number 'BIBE1CBEX1-2026-E-10502026' has already been submitted. _(DUPLICATE_CUSDEC)_",
		}, Reasons(b))
	})

	t.Run("missing client key", func(t *testing.T) {
		b := body(t, `{
			"error": {
				"code": "MISSING_CLIENT_HEADER",
				"message": "Client identifier header 'slpacmsuser-key' is required."
			},
			"status": 0,
			"openapi": "3.0.3"
		}`)

		assert.Equal(t, []string{
			"- Client identifier header 'slpacmsuser-key' is required. _(MISSING_CLIENT_HEADER)_",
		}, Reasons(b))
	})
}

// A validation failure says only "the given request data was invalid" in its
// message and names the fields in details, so the details are what is shown.
func TestReasons_ValidationFailure(t *testing.T) {
	b := body(t, `{
		"error": {
			"code": "VALIDATION_FAILED",
			"details": {
				"containers.0.cbm": ["The containers.0.cbm must be at least 0.01."],
				"containers.0.commodity": ["The selected containers.0.commodity is invalid."]
			},
			"message": "The given request data was invalid."
		},
		"status": 0,
		"openapi": "3.0.3"
	}`)

	require.True(t, HasErrors(b))
	// Sorted by field, so the same refusal reads the same way twice.
	assert.Equal(t, []string{
		"- The containers.0.cbm must be at least 0.01. _(containers.0.cbm)_",
		"- The selected containers.0.commodity is invalid. _(containers.0.commodity)_",
	}, Reasons(b))
	assert.NotContains(t, Reasons(b), "The given request data was invalid.",
		"the summary adds nothing once the fields are named")
}

// A field can carry more than one reason — a service both unknown and wrong for
// the commodity reports both — and every field in the refusal must be rendered.
func TestReasons_ValidationFailureWithManyFields(t *testing.T) {
	b := body(t, `{
		"error": {
			"code": "VALIDATION_FAILED",
			"details": {
				"cusdec_id": ["The selected cusdec id is invalid."],
				"cusdec_no": ["The selected cusdec no is invalid."],
				"containers.0.cbm": ["The containers.0.cbm must be at least 0.01."],
				"parent_invoice_no": ["The parent invoice 'string' was not found."],
				"containers.0.service_id": [
					"The selected containers.0.service_id is invalid.",
					"Dangerous Cargo (DC) commodity can only use service DANGEROUS CARGO (sqid: xrw3cfz8)."
				],
				"containers.0.container_id": ["The containers.0.container_id must be an integer."]
			},
			"message": "The given request data was invalid."
		},
		"status": 0,
		"openapi": "3.0.3"
	}`)

	reasons := Reasons(b)
	assert.Len(t, reasons, 7, "six fields, one of them with two reasons")
	assert.Equal(t, []string{
		"- The containers.0.cbm must be at least 0.01. _(containers.0.cbm)_",
		"- The containers.0.container_id must be an integer. _(containers.0.container_id)_",
		"- The selected containers.0.service_id is invalid. _(containers.0.service_id)_",
		"- Dangerous Cargo (DC) commodity can only use service DANGEROUS CARGO (sqid: xrw3cfz8). _(containers.0.service_id)_",
		"- The selected cusdec id is invalid. _(cusdec_id)_",
		"- The selected cusdec no is invalid. _(cusdec_no)_",
		"- The parent invoice 'string' was not found. _(parent_invoice_no)_",
	}, reasons)
}

// Nothing to say is reported as nothing, so the caller can fall back to its own
// wording rather than showing an empty rejection.
func TestReasons_NothingToSay(t *testing.T) {
	for name, response := range map[string]string{
		"no error at all":     `{"data": {"status": "ACCEPTED"}, "status": 1}`,
		"an empty error":      `{"error": {}, "status": 0}`,
		"a message-less code": `{"error": {"code": "SOMETHING"}, "status": 0}`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, Reasons(body(t, response)))
		})
	}

	t.Run("an empty error object is not a refusal", func(t *testing.T) {
		assert.False(t, HasErrors(body(t, `{"error": {}, "status": 0}`)))
	})
}

// The key is an opaque string SLPA issues per company, mapped from the company
// profile (company.data.slpacmsuser_key). A value of any other shape is no key
// at all: rendering one from it would file the submission against a company
// nobody chose.
func TestClientKeyHeaders(t *testing.T) {
	assert.Equal(t, map[string]string{ClientKeyHeader: "agztNvLSUA"},
		ClientKeyHeaders(map[string]any{ClientKeyInput: " agztNvLSUA \n"}, "slpa test"))

	for name, inputs := range map[string]map[string]any{
		"no key mapped in":  {},
		"absent":            {ClientKeyInput: nil},
		"blank":             {ClientKeyInput: "   "},
		"a number":          {ClientKeyInput: 42},
		"the whole profile": {ClientKeyInput: map[string]any{"slpacmsuser_key": "agztNvLSUA"}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, ClientKeyHeaders(inputs, "slpa test"))
		})
	}
}

// A field the CMS did not send must not appear on the task as an empty one.
func TestCapture(t *testing.T) {
	body := map[string]any{"status": "ACCEPTED", "reference": "ECDN-1", "extra": 1}

	assert.Equal(t, map[string]any{"status": "ACCEPTED", "reference": "ECDN-1"},
		Capture(body, "status", "reference", "cusdec_serial"))
	assert.Empty(t, Capture(map[string]any{}, "status"))
}

func TestFailure(t *testing.T) {
	const intro, outro = "SLPA did not accept it:", "\n\nTry again."

	refused := Failure(nil, map[string]any{"error": map[string]any{
		"code": "INVALID", "details": map[string]any{"cusdecNo": []any{"is required"}}}}, intro, outro)
	assert.Contains(t, refused, "is required", "the CMS's own reason wins")

	// Nothing came back at all: neither a reason to quote nor a cause we have
	// established, so the trader is told only what is known.
	assert.Equal(t, Unreachable, Failure(errors.New("dial tcp: timeout"), nil, intro, outro))

	// An answer with no reason in it is still an answer.
	assert.Equal(t, intro+outro, Failure(nil, map[string]any{"status": 0}, intro, outro))
}
