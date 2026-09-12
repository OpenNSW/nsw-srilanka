package invoice

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SLPA's own answer to the generate-invoice call, kept as they send it: the
// figures the trader pays against sit two levels down, under data.details, and
// the envelope repeats only the number.
const issued = `{
  "openapi": "3.0.3",
  "status": 1,
  "data": {
    "invoice_no": "26211843261345",
    "service_order_no": "SO-FCL-EXPORT-2026-262342",
    "so_status": "client_generate_invoice",
    "is_paid": false,
    "details": {
      "draft_invoice_no": "DRFT-INV-FCL-EXPORT-2026-257603",
      "invoice_no": "26211843261345",
      "invoice_serial": "26SEP_LD1_00000017",
      "status": "client_generate_invoice",
      "total_usd": 16,
      "total_lkr": 4776,
      "exchange_rate": 298.5,
      "total_payable_lkr": 4776,
      "vat_lkr": 0,
      "invoice_url": null,
      "invoice_generated_at": "2026-09-11 15:35:40",
      "invoice_paid_at": null,
      "payment_slip_url": null,
      "items": [{"container_no": "MSCU8492019", "total_lkr": 4776}]
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
	assert.Equal(t, "26211843261345", out["invoice_no"])
	assert.Equal(t, "26SEP_LD1_00000017", out["invoice_serial"])
	assert.Equal(t, "SO-FCL-EXPORT-2026-262342", out["service_order_no"])
	assert.Equal(t, 4776.0, out["payable_lkr"])
	assert.Equal(t, 298.5, out["exchange_rate"])
	assert.Equal(t, "2026-09-11 15:35:40", out["generated_at"])
	assert.Equal(t, false, out["paid"], "raising the invoice does not settle it")
}

// The CMS prices the order in dollars and converts. What leaves the trader's
// account is the rupee figure, so the dollar one is not carried to the panel at
// all: a trader reading both could transfer the wrong number.
func TestGenerate_CarriesNoDollarFigure(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	for key, value := range out {
		assert.NotEqual(t, 16.0, value, "the USD subtotal reached the panel as %q", key)
	}
	assert.NotContains(t, out, "total_usd")
}

// A field the CMS sent as null is left off rather than recorded empty: the panel
// hides what is absent, and an empty link reads as a document that failed to
// arrive rather than one not issued yet.
func TestGenerate_OmitsWhatTheCMSHasNotIssuedYet(t *testing.T) {
	_, out := NewGenerateInterpreter().Interpret(nil, body(t, issued))

	assert.NotContains(t, out, "payment_slip_url")
	assert.NotContains(t, out, "invoice_url")
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
	  "data": {"invoice_no": "26211843261345", "is_paid": true,
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
