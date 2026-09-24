package invoice

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SLPA's own answer to the generate-invoice call, kept as they send it: the
// figures the trader pays against sit two levels down under data.details, the
// breakdown is one entry per container, and the link that can actually be paid
// against hangs off the payment_slip block rather than beside it.
const issued = `{
  "openapi": "3.0.3",
  "status": 1,
  "data": {
    "invoice_no": null,
    "service_order_no": "SO-FCL-EXPORT-2026-262351",
    "so_status": "client_generate_invoice",
    "is_paid": false,
    "details": {
      "draft_invoice_no": "DRFT-INV-FCL-EXPORT-2026-257612",
      "invoice_no": "26211843262217",
      "invoice_serial": "26SEP_LD1_00000026",
      "status": "client_generate_invoice",
      "total_usd": 132,
      "total_lkr": 39402,
      "exchange_rate": 298.5,
      "total_payable_lkr": 39402,
      "invoice_url": "https://slpacargoapi.slpa.lk/pdf/invoice/26211843262217?signature=82f46b01",
      "invoice_generated_at": "2026-09-15T13:46:42+05:30",
      "invoice_paid_at": null,
      "items": [
        {"container_no": "MSCU8492019", "container_size": "20", "container_type": "general",
         "service_name": "COCONUT OIL THROUGH OTHER FORM", "quantity": "1",
         "total_lkr": 4776, "total_usd": 16},
        {"container_no": "3426566", "container_size": "40", "container_type": "general",
         "service_name": "DANGEROUS CARGO", "quantity": "1",
         "total_lkr": 34626, "total_usd": 116}
      ],
      "payment_slip": {
        "date": "2026-09-15",
        "time": "13:46:43",
        "total": "39,402",
        "number": "BIBE1E40452026",
        "shipper": "JOTHI COCONUT EXPORTERS",
        "consignee": "TSNW Test user",
        "invoice_no": "26211843262217",
        "number_type": "CusDec",
        "payment_slip_url": "https://slpacargoapi.slpa.lk/pdf/payment-slip/26211843262217?signature=18ce9a02"
      }
    }
  }
}`

func body(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

// What the trader is shown, and what the step after this one needs.
func TestGenerate_RecordsWhatTheTraderPaysAgainst(t *testing.T) {
	ok, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	require.True(t, ok)
	assert.Equal(t, "26211843262217", out["invoice_no"])
	assert.Equal(t, "26SEP_LD1_00000026", out["invoice_serial"])
	assert.Equal(t, "SO-FCL-EXPORT-2026-262351", out["service_order_no"])
	assert.Equal(t, 39402.0, out["payable_lkr"])
	assert.Equal(t, 298.5, out["exchange_rate"])
	assert.Equal(t, "2026-09-15T13:46:42+05:30", out["generated_at"])
	assert.Equal(t, false, out["paid"], "raising the invoice does not settle it")
}

// The CMS prices the order in dollars and converts. What leaves the trader's
// account is the rupee figure, so the dollar one is not carried to the panel at
// all: a trader reading both could transfer the wrong number.
func TestGenerate_CarriesNoDollarFigure(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	for key, value := range out {
		assert.NotEqual(t, 132.0, value, "the USD total reached the panel as %q", key)
	}
	assert.NotContains(t, out, "total_usd")
}

// The payment slip link hangs off the payment_slip block. Read from beside it,
// as this once was, it silently resolves to nothing and the trader is shown an
// invoice with no way to pay it.
func TestGenerate_ReadsThePaymentSlipLinkFromTheSlip(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	assert.Equal(t,
		"https://slpacargoapi.slpa.lk/pdf/payment-slip/26211843262217?signature=18ce9a02",
		out["payment_slip_url"])
}

// One link, and it is the one that can be paid against. The invoice document is
// not carried beside it: offering both is how a trader transfers against the
// wrong one.
func TestGenerate_CarriesOnlyThePayableLink(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	assert.NotContains(t, out, "invoice_url")
}

// The invoice is broken down the way SLPA priced it: one entry per container,
// which is the unit charged for and the one a trader checks the total against.
func TestGenerate_BreaksTheInvoiceDownByContainer(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	items, ok := out["items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	assert.Equal(t, "MSCU8492019", items[0]["container_no"])
	assert.Equal(t, "COCONUT OIL THROUGH OTHER FORM", items[0]["service_name"])
	assert.Equal(t, "20", items[0]["container_size"])
	assert.Equal(t, 4776.0, items[0]["total_lkr"])

	assert.Equal(t, "3426566", items[1]["container_no"])
	assert.Equal(t, "DANGEROUS CARGO", items[1]["service_name"])
	assert.Equal(t, 34626.0, items[1]["total_lkr"])

	// The breakdown has to add up to what is being asked for, or it is worse
	// than no breakdown at all.
	assert.Equal(t, out["payable_lkr"], items[0]["total_lkr"].(float64)+items[1]["total_lkr"].(float64))
}

// What the bank is handed: who is paying, against which reference, for how
// much. Stated on the panel so the trader does not have to open the PDF to
// learn what they are transferring.
func TestGenerate_RecordsThePaymentSlip(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	slip, ok := out["payment_slip"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "BIBE1E40452026", slip["number"])
	assert.Equal(t, "CusDec", slip["number_type"])
	assert.Equal(t, "JOTHI COCONUT EXPORTERS", slip["shipper"])
	assert.Equal(t, "TSNW Test user", slip["consignee"])
	assert.Equal(t, "39,402", slip["total"])
	assert.Equal(t, "2026-09-15", slip["date"])
}

// The invoice number is what says an invoice exists. An answer without one is
// not an issue, whatever else it carried.
func TestGenerate_AnAnswerWithNoNumberIsNotAnInvoice(t *testing.T) {
	ok, out := NewGenerateInterpreter().Interpret(nil, body(t, `{"status": 1, "data": {"so_status": "client_new_actclk"}}`))

	require.False(t, ok)
	assert.Contains(t, out["error"], "SLPA did not issue the invoice")
}

// A refusal is reported in SLPA's own words, since only they know why.
func TestGenerate_RefusalCarriesTheCMSsOwnReason(t *testing.T) {
	ok, out := NewGenerateInterpreter().Interpret(nil,
		body(t, `{"status": 0, "message": "Service order is not approved yet"}`))

	require.False(t, ok)
	assert.Contains(t, out["error"], "Service order is not approved yet")
}

// A call that never arrived says so plainly rather than naming a cause nobody
// established.
func TestGenerate_UnreachableCMSSaysSo(t *testing.T) {
	ok, out := NewGenerateInterpreter().Interpret(errors.New("dial tcp: timeout"), map[string]any{})

	require.False(t, ok)
	assert.Contains(t, out["error"], "SLPA")
}

// An order already invoiced and paid must not send the trader to wait for a
// payment that has happened.
func TestGenerate_ReadsAnInvoiceAlreadyPaid(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, `{
	  "status": 1,
	  "data": {"is_paid": true,
	           "details": {"invoice_no": "26211843261345", "total_payable_lkr": 4776,
	                       "invoice_paid_at": "2026-09-11 16:02:11"}}
	}`))

	assert.Equal(t, true, out["paid"])
}

// The client key identifies the company the invoice is raised for.
func TestGenerate_PresentsTheClientKey(t *testing.T) {
	headers := NewGenerateInterpreter().BuildHeaders(map[string]any{"client_key": "agztNvLSUA"})
	assert.Equal(t, "agztNvLSUA", headers["slpacmsuser-key"])
}

// The service order is named in the path and the company in a header, so the
// call posts nothing — as SLPA's own published request does.
func TestGenerate_SendsNoBody(t *testing.T) {
	assert.Nil(t, NewGenerateInterpreter().BuildRequest(map[string]any{"service_order_no": "SO-FCL-EXPORT-2026-262342"}))
}
