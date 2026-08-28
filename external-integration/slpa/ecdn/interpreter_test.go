package ecdn

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpreter_BuildRequest(t *testing.T) {
	// Encode is what the transport calls, so asserting on it is asserting on the
	// bytes SLPA receives — and on the Content-Type that describes them.
	body, contentType, err := NewInterpreter().
		BuildRequest(map[string]any{"payload": fullForm()}).
		Encode()
	require.NoError(t, err)
	assert.Equal(t, "application/json", contentType)

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

// SLPA identifies the submitting company by this header, and the key reaches the
// interpreter as a task input the artifact maps from the company profile.
func TestInterpreter_BuildHeaders(t *testing.T) {
	i := NewInterpreter()

	t.Run("the key is presented as the header", func(t *testing.T) {
		assert.Equal(t, map[string]string{ClientKeyHeader: "agztNvLSUA"},
			i.BuildHeaders(map[string]any{ClientKeyInput: "agztNvLSUA"}))
	})

	t.Run("surrounding space is trimmed", func(t *testing.T) {
		assert.Equal(t, map[string]string{ClientKeyHeader: "agztNvLSUA"},
			i.BuildHeaders(map[string]any{ClientKeyInput: "  agztNvLSUA \n"}))
	})

	// Sending nothing lets SLPA say the caller cannot be identified, which is a
	// truer message than one invented here.
	for name, inputs := range map[string]map[string]any{
		"no key mapped in": {},
		"a blank key":      {ClientKeyInput: "   "},
		"not a string":     {ClientKeyInput: 42},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, i.BuildHeaders(inputs))
		})
	}
}

// The CMS's answer to a declaration it stored: the outcome nested under "data",
// the envelope's own status on top.
func TestInterpreter_InterpretAccepted(t *testing.T) {
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
	// how SLPA refers to it afterwards, and what a service order is raised
	// against.
	assert.Equal(t, "BIBE1CBEX1-2026-E-10512026", out["cusdec_serial"])
	assert.Equal(t, "2026-08-22T18:13:57+05:30", out["validated_at"])
	assert.Equal(t, "ACCEPTED", out["status"], "the nested outcome wins over the envelope's status")
	assert.NotContains(t, out, "error")
}

func TestInterpreter_InterpretRejections(t *testing.T) {
	i := NewInterpreter()

	// A declaration the CMS already holds. Its own wording names the serial, so
	// the trader can see which submission it collided with.
	t.Run("a duplicate declaration", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
			"error": map[string]any{
				"code":    "DUPLICATE_CUSDEC",
				"message": "CUSDEC serial number 'BIBE1CBEX1-2026-E-10502026' has already been submitted.",
			},
			"status":  float64(0),
			"openapi": "3.0.3",
		})
		assert.False(t, accepted)

		msg := out["error"].(string)
		assert.Contains(t, msg, "did not accept your cargo declaration")
		assert.Contains(t, msg, "has already been submitted.")
		assert.Contains(t, msg, "_(DUPLICATE_CUSDEC)_")
	})

	// A call the CMS cannot attribute to a company: the client key never left
	// here. Their message is better than one invented locally, so it is shown.
	t.Run("no client key on the call", func(t *testing.T) {
		_, out := i.Interpret(nil, map[string]any{
			"error": map[string]any{
				"code":    "MISSING_CLIENT_HEADER",
				"message": "Client identifier header 'slpacmsuser-key' is required.",
			},
			"status": float64(0),
		})
		assert.Contains(t, out["error"], "Client identifier header 'slpacmsuser-key' is required. _(MISSING_CLIENT_HEADER)_")
	})

	// A validation failure names the fields; the summary in "message" adds
	// nothing once they are listed.
	t.Run("a validation failure lists its fields", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
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
		assert.False(t, accepted)

		msg := out["error"].(string)
		assert.Contains(t, msg, "The containers.0.cbm must be at least 0.01. _(containers.0.cbm)_")
		assert.Contains(t, msg, "The selected containers.0.commodity is invalid. _(containers.0.commodity)_")
	})

	// The envelope's status is not a verdict, so a body with no outcome in it is
	// a refusal rather than an acceptance.
	t.Run("an envelope status alone is not an acceptance", func(t *testing.T) {
		accepted, _ := i.Interpret(nil, map[string]any{"status": float64(1), "openapi": "3.0.3"})
		assert.False(t, accepted)
	})

	// A refusal we cannot read a reason from still must not read as a blank
	// rejection: the trader is told the declaration was not accepted.
	t.Run("a refusal with no reason in it", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{"error": map[string]any{"code": "SOMETHING"}, "status": float64(0)})
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "did not accept your cargo declaration")
	})

	t.Run("transport failure", func(t *testing.T) {
		accepted, out := i.Interpret(errors.New("dial tcp: timeout"), nil)
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "could not get a usable answer")
	})
}

// A declaration that cannot be assembled must not be uploaded. Sending an empty
// one would put a meaningless document in front of a live cargo system, and
// answer the trader with SLPA's opinion of it rather than the real fault.
func TestBuildRequest_DoesNotSendWhatItCouldNotAssemble(t *testing.T) {
	formWithoutContainers := fullForm()
	delete(formWithoutContainers, "containers")

	for name, inputs := range map[string]map[string]any{
		"no form at all":            {},
		"form of wrong type":        {"payload": "not a form"},
		"a form with no containers": {"payload": formWithoutContainers},
	} {
		t.Run(name, func(t *testing.T) {
			body := NewInterpreter().BuildRequest(inputs)
			require.NotNil(t, body)

			data, _, err := body.Encode()
			require.Error(t, err, "the body must refuse to encode, so nothing is sent")
			assert.ErrorIs(t, err, ErrNotAssembled)
			assert.Empty(t, data)
			assert.NotContains(t, string(data), "xml_payload")
		})
	}
}

// The trader is told what happened here, not what SLPA thinks of a document it
// never received.
func TestInterpret_ReportsALocalAssemblyFailureAsOurs(t *testing.T) {
	body := NewInterpreter().BuildRequest(map[string]any{"payload": map[string]any{}})
	_, _, callErr := body.Encode()
	require.Error(t, callErr)

	accepted, out := NewInterpreter().Interpret(callErr, nil)

	require.False(t, accepted)
	message, _ := out["error"].(string)
	assert.Contains(t, message, "could not be prepared for SLPA, so nothing was submitted")
	assert.NotContains(t, message, "SLPA did not accept",
		"SLPA refused nothing; they never saw it")
	assert.NotContains(t, message, ErrNotAssembled.Error(),
		"the sentinel is for code, not for the trader")
}

// The key is an opaque string SLPA issues per company, mapped from the company
// profile. A value of any other shape is no key at all: rendering one from it
// would file the declaration against a company nobody chose.
func TestBuildHeaders_RequiresAStringKey(t *testing.T) {
	i := NewInterpreter()
	assert.Equal(t, map[string]string{ClientKeyHeader: "agztNvLSUA"},
		i.BuildHeaders(map[string]any{ClientKeyInput: " agztNvLSUA "}))

	for name, value := range map[string]any{
		"absent":            nil,
		"blank":             "   ",
		"a number":          42,
		"the whole profile": map[string]any{"slpacmsuser_key": "agztNvLSUA"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, i.BuildHeaders(map[string]any{ClientKeyInput: value}))
		})
	}
}
