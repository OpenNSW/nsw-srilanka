package serviceorder

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"

	"github.com/OpenNSW/core/remote"
)

// sundryForm is the sundry form as the engine hands it over: the parent invoice
// the trader entered and a sundry service chosen against each container.
func sundryForm() map[string]any {
	return map[string]any{
		"parentInvoiceNo": "INV-FCL-EXPORT-2026-000123",
		"lines": []any{
			map[string]any{"containerNo": "MSCU8492019", "serviceId": "l1c73wk5", "quantity": float64(1)},
			map[string]any{"containerNo": "MSCU8492020", "serviceId": "74ura32u", "quantity": float64(2)},
		},
		"additionalNote": "Crane needed at berth 3",
	}
}

// builtSundry returns the order the sundry interpreter would send, unwrapped
// from the body the transport asks for.
func builtSundry(t *testing.T, form map[string]any) SundryRequest {
	t.Helper()
	inputs := map[string]any{}
	if form != nil {
		inputs["payload"] = form
	}
	body, ok := NewSundryInterpreter().BuildRequest(inputs).(remote.JSONBody)
	require.True(t, ok, "the order is sent as JSON")
	req, ok := body.V.(SundryRequest)
	require.True(t, ok, "the body carries the sundry order")
	return req
}

// A sundry order names the parent invoice instead of a declaration and is
// flagged as sundry; each line carries only the container, service and quantity.
func TestSundryBuildRequest(t *testing.T) {
	body, err := json.Marshal(builtSundry(t, sundryForm()))
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"parent_invoice_no": "INV-FCL-EXPORT-2026-000123",
		"sundry_invoice": true,
		"additional_note": "Crane needed at berth 3",
		"containers": [
			{"container_no": "MSCU8492019", "service_id": "l1c73wk5", "quantity": 1, "lcl_status": false},
			{"container_no": "MSCU8492020", "service_id": "74ura32u", "quantity": 2, "lcl_status": false}
		]
	}`, string(body))
}

// A JSONForms integer widget can round-trip as a string; the quantity the trader
// entered must still reach the CMS.
func TestSundryBuildRequest_QuantitySubmittedAsAString(t *testing.T) {
	form := sundryForm()
	form["lines"].([]any)[0].(map[string]any)["quantity"] = "3"

	req := builtSundry(t, form)
	require.NotEmpty(t, req.Containers)
	assert.Equal(t, 3, req.Containers[0].Quantity)
}

// The contract cannot fail a call, so an order that will not assemble goes out
// with no lines and the CMS answers it. What must not happen is a half-built
// order being raised.
func TestSundryBuildRequest_SendsNothingItCannotAssemble(t *testing.T) {
	t.Run("no form in the inputs", func(t *testing.T) {
		assert.Equal(t, SundryRequest{SundryInvoice: true}, builtSundry(t, nil))
	})

	t.Run("no parent invoice", func(t *testing.T) {
		form := sundryForm()
		delete(form, "parentInvoiceNo")

		assert.Equal(t, SundryRequest{SundryInvoice: true}, builtSundry(t, form))
	})

	for name, edit := range map[string]func(line map[string]any){
		"no lines":        nil,
		"no container":    func(line map[string]any) { delete(line, "containerNo") },
		"no service":      func(line map[string]any) { delete(line, "serviceId") },
		"zero quantity":   func(line map[string]any) { line["quantity"] = float64(0) },
		"no quantity":     func(line map[string]any) { delete(line, "quantity") },
		"blank container": func(line map[string]any) { line["containerNo"] = "  " },
	} {
		t.Run(name, func(t *testing.T) {
			form := sundryForm()
			if edit == nil {
				form["lines"] = []any{}
			} else {
				edit(form["lines"].([]any)[1].(map[string]any))
			}

			req := builtSundry(t, form)
			assert.Empty(t, req.Containers, "no line of a half-built order is sent")
			// The parent invoice is still named, so the CMS's refusal is about the
			// order rather than about a request it cannot place at all.
			assert.Equal(t, "INV-FCL-EXPORT-2026-000123", req.ParentInvoiceNo)
			assert.True(t, req.SundryInvoice)
		})
	}
}

// The validation messages name the line a trader has to fix.
func TestBuildSundry_MessagesNameTheLine(t *testing.T) {
	form := sundryForm()
	delete(form["lines"].([]any)[1].(map[string]any), "serviceId")

	_, err := BuildSundry(form)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MSCU8492020")
}

// SLPA identifies the submitting company by the same header on a sundry order.
func TestSundryInterpreter_BuildHeaders(t *testing.T) {
	assert.Equal(t, map[string]string{cms.ClientKeyHeader: "agztNvLSUA"},
		NewSundryInterpreter().BuildHeaders(map[string]any{cms.ClientKeyInput: "agztNvLSUA"}))
	assert.Nil(t, NewSundryInterpreter().BuildHeaders(map[string]any{}))
}

func TestSundryInterpret(t *testing.T) {
	i := NewSundryInterpreter()

	t.Run("raised", func(t *testing.T) {
		ok, out := i.Interpret(nil, map[string]any{"data": map[string]any{
			"service_order_no":  "SO-FCL-EXPORT-2026-000124",
			"slug":              "so-fcl-export-2026-000124",
			"parent_invoice_no": "INV-FCL-EXPORT-2026-000123",
		}})
		assert.True(t, ok)
		assert.Equal(t, "SO-FCL-EXPORT-2026-000124", out["service_order_no"])
		assert.Equal(t, "so-fcl-export-2026-000124", out["slug"])
		assert.Equal(t, "INV-FCL-EXPORT-2026-000123", out["parent_invoice_no"])
		assert.NotContains(t, out, "error")
	})

	t.Run("refused", func(t *testing.T) {
		ok, out := i.Interpret(nil, map[string]any{"error": map[string]any{
			"code": "VALIDATION", "message": "parent_invoice_no is invalid",
		}})
		assert.False(t, ok)
		assert.Contains(t, out["error"], "SLPA did not raise your sundry order:")
	})

	// No answer to report, so the trader is told to retry rather than shown a
	// refusal the CMS never made.
	t.Run("transport failure", func(t *testing.T) {
		ok, out := i.Interpret(errors.New("connection refused"), nil)
		assert.False(t, ok)
		assert.NotEmpty(t, out["error"])
	})
}
