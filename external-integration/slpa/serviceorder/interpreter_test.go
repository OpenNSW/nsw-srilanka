package serviceorder

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/remote"
)

// fclForm is the service-selection form as the engine hands it over: the
// declaration's containers, echoed from the accepted ECDN, with a service chosen
// against each.
func fclForm() map[string]any {
	return map[string]any{
		"cusdecNo":  "BIBE1CBEX1-2026-E-10532026",
		"cargoType": "FCL",
		"containers": []any{
			map[string]any{
				"containerNo":   "MSCU8492019",
				"containerType": "General",
				"containerSize": "20",
				"commodity":     "DC/Reefer/Liquor",
				"cbm":           32.5,
				"serviceId":     "d0n6oeqw",
			},
		},
		"additionalNote": "Handle with care",
	}
}

// built returns the order the interpreter would send, unwrapped from the body the
// transport asks for.
func built(t *testing.T, form map[string]any) Request {
	t.Helper()
	body, ok := NewInterpreter().BuildRequest(map[string]any{"payload": form}).(remote.JSONBody)
	require.True(t, ok, "the order is sent as JSON")
	req, ok := body.V.(Request)
	require.True(t, ok, "the body carries the order")
	return req
}

func TestBuildRequest_FCL(t *testing.T) {
	body, err := json.Marshal(built(t, fclForm()))
	require.NoError(t, err)

	// The declaration is named, the cargo type is not sent (the CMS derives it),
	// and the order is standard rather than sundry.
	assert.JSONEq(t, `{
		"cusdec_no": "BIBE1CBEX1-2026-E-10532026",
		"sundry_invoice": false,
		"additional_note": "Handle with care",
		"containers": [{
			"container_no": "MSCU8492019",
			"container_type": "General",
			"container_size": "20",
			"service_id": "d0n6oeqw",
			"quantity": 1,
			"cbm": 32.5,
			"lcl_status": false
		}]
	}`, string(body))
}

// Only FCL declarations can raise an order for now, so a loose-cargo declaration
// is refused rather than ordered as though it were containerised.
func TestBuildRequest_LCLIsRefused(t *testing.T) {
	form := fclForm()
	form["cargoType"] = "LCL"

	req := built(t, form)
	assert.Empty(t, req.Containers, "no lines are ordered for cargo this cannot order for")
	// The declaration is still named, so the CMS's refusal is about the order
	// rather than about a request it cannot place at all.
	assert.Equal(t, "BIBE1CBEX1-2026-E-10532026", req.CusdecNo)
}

// A container size the form submitted as a number still reaches the CMS as the
// string its schema asks for.
func TestBuildRequest_NumericContainerSize(t *testing.T) {
	form := fclForm()
	form["containers"].([]any)[0].(map[string]any)["containerSize"] = float64(40)

	req := built(t, form)
	assert.Equal(t, "40", req.Containers[0].ContainerSize)
}

// The contract cannot fail a call, so an order that will not assemble goes out
// with no lines and the CMS answers it. What must not happen is a half-built
// order being raised.
func TestBuildRequest_SendsNothingItCannotAssemble(t *testing.T) {
	t.Run("no form in the inputs", func(t *testing.T) {
		assert.Equal(t, Request{}, built(t, nil))
	})

	t.Run("no service chosen", func(t *testing.T) {
		form := fclForm()
		delete(form["containers"].([]any)[0].(map[string]any), "serviceId")

		req := built(t, form)
		assert.Empty(t, req.Containers)
		// The declaration is still named, so the CMS's refusal is about the order
		// rather than about a request it cannot place at all.
		assert.Equal(t, "BIBE1CBEX1-2026-E-10532026", req.CusdecNo)
	})

	// The CMS refuses a line with no volume ("must be at least 0.01"), so it is
	// caught here where the message can name the container.
	t.Run("no volume on a container", func(t *testing.T) {
		form := fclForm()
		delete(form["containers"].([]any)[0].(map[string]any), "cbm")

		req := built(t, form)
		assert.Empty(t, req.Containers)
	})

	t.Run("no declaration serial", func(t *testing.T) {
		form := fclForm()
		delete(form, "cusdecNo")

		assert.Equal(t, Request{}, built(t, form))
	})
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

func TestInterpret(t *testing.T) {
	i := NewInterpreter()

	// The response the CMS actually gives: the order's own workflow state, not a
	// verdict, and no invoice yet. The order number is what says it exists.
	t.Run("raised", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
			"data": map[string]any{
				"slug":              "8d326f3a-643a-4a1d-8072-87130288b032",
				"reason":            nil,
				"status":            "client_new_actclk",
				"total_lkr":         float64(0),
				"total_usd":         float64(16),
				"created_at":        "2026-08-24T22:49:47+05:30",
				"invoice_no":        nil,
				"cusdec_serial":     "BIBE1CBEX1-2026-E-10642026",
				"service_order_no":  "SO-FCL-EXPORT-2026-262316",
				"parent_invoice_no": nil,
			},
			"status":  float64(1),
			"openapi": "3.0.3",
		})
		assert.True(t, accepted, "an order with a number was raised, whatever state it is in")
		assert.Equal(t, "SO-FCL-EXPORT-2026-262316", out["service_order_no"])
		assert.Equal(t, "client_new_actclk", out["status"])
		assert.Equal(t, float64(16), out["total_usd"])
		assert.Equal(t, "8d326f3a-643a-4a1d-8072-87130288b032", out["slug"])
		assert.NotContains(t, out, "error")
	})

	// An endpoint that does answer with a plain verdict is still honoured, so an
	// order number is a second way of recognising one rather than the only way.
	t.Run("raised with an accepted status and no number", func(t *testing.T) {
		accepted, _ := i.Interpret(nil, map[string]any{"data": map[string]any{"status": "ACCEPTED"}})
		assert.True(t, accepted)
	})

	// An order number in a body that also carries a refusal is not an acceptance.
	t.Run("a number alongside a refusal is still a refusal", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
			"data":  map[string]any{"service_order_no": "SO-2026-00001"},
			"error": map[string]any{"code": "VALIDATION_FAILED", "message": "The given request data was invalid."},
		})
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "The given request data was invalid.")
	})

	// The refusal this endpoint gives most often, and the reason a trader can act
	// on: the CMS pairs a special commodity with one service and names the rule.
	t.Run("a validation failure names the field and the rule", func(t *testing.T) {
		accepted, out := i.Interpret(nil, map[string]any{
			"error": map[string]any{
				"code":    "VALIDATION_FAILED",
				"message": "The given request data was invalid.",
				"details": map[string]any{
					"containers.0.service_id": []any{
						"Dangerous Cargo (DC) commodity can only use service DANGEROUS CARGO (sqid: xrw3cfz8).",
					},
				},
			},
			"status": float64(0),
		})
		assert.False(t, accepted)

		msg := out["error"].(string)
		assert.Contains(t, msg, "did not raise your service order")
		assert.Contains(t, msg, "can only use service DANGEROUS CARGO")
		assert.Contains(t, msg, "_(containers.0.service_id)_")
	})

	// An envelope code is not a verdict, so a body with no outcome is a rejection.
	t.Run("an envelope status alone is not an acceptance", func(t *testing.T) {
		accepted, _ := i.Interpret(nil, map[string]any{"status": float64(1), "openapi": "3.0.3"})
		assert.False(t, accepted)
	})

	t.Run("transport failure", func(t *testing.T) {
		accepted, out := i.Interpret(errors.New("dial tcp: timeout"), nil)
		assert.False(t, accepted)
		assert.Contains(t, out["error"], "could not get a usable answer")
	})
}
