package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handlerOver builds the handler over a store that answers every lookup the way
// the test wants, since one signed route carries both lifecycles.
func handlerOver(t *testing.T, parked bool) (*Handler, *orderCompleter) {
	t.Helper()

	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	mock.MatchExpectationsInOrder(false)
	for i := 0; i < 4; i++ {
		if parked {
			mock.ExpectQuery(`SELECT "task_id" FROM "task_records_v2"`).
				WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-1"))
			continue
		}
		mock.ExpectQuery(`SELECT "task_id" FROM "task_records_v2"`).WillReturnError(gorm.ErrRecordNotFound)
		mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).WillReturnRows(sqlmock.NewRows([]string{"state"}))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "task_records_v2"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}

	tasks := &orderCompleter{}
	h, err := NewHandler(NewOrderEvents(db, tasks), NewInvoiceEvents(db, tasks), Config{Secret: secret})
	require.NoError(t, err)
	return h, tasks
}

func post(t *testing.T, h *Handler, body string, signature string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slpa", strings.NewReader(body))
	if signature != "" {
		req.Header.Set(SignatureHeader, signature)
	}
	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, req)
	return rec
}

const approvedBody = `{
	"event": "service_order.approved_by_accountant",
	"slug": "8d326f3a-643a-4a1d-8072-87130288b032",
	"service_order_no": "SO-FCL-EXPORT-2026-262316",
	"cusdec_serial": "BIBE1CBEX1-2026-E-10642026",
	"status": "act_approve",
	"invoice_no": "INV-2026-04412",
	"total_amount": 16,
	"timestamp": "2026-08-26T02:30:59.846Z"
}`

func TestHandleWebhook_AppliesASignedDecision(t *testing.T) {
	h, tasks := handlerOver(t, true)
	rec := post(t, h, approvedBody, Sign([]byte(approvedBody), secret))
	assert.Equal(t, http.StatusOK, rec.Code)

	// The decision reached the task manager, carrying what this integration acts
	// on; the rest of their payload is theirs to hold.
	require.True(t, tasks.called)
	assert.Equal(t, DecisionApproved, tasks.payload["decision"])
	assert.Equal(t, true, tasks.payload["final"])
	assert.Equal(t, "SO-FCL-EXPORT-2026-262316", tasks.payload["service_order_no"])
	assert.Equal(t, "INV-2026-04412", tasks.payload["invoice_no"])
}

// This route has no bearer token in front of it, so an unsigned or wrongly
// signed call must never reach the service — advancing another trader's
// consignment is exactly what the signature is there to prevent.
func TestHandleWebhook_RefusesWhatItCannotAuthenticate(t *testing.T) {
	for name, signature := range map[string]string{
		"unsigned":                  "",
		"signed by someone else":    Sign([]byte(approvedBody), "not-the-secret"),
		"signature of another body": Sign([]byte(`{"event":"service_order.rejected_by_accountant"}`), secret),
	} {
		t.Run(name, func(t *testing.T) {
			h, tasks := handlerOver(t, true)
			rec := post(t, h, approvedBody, signature)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.False(t, tasks.called, "nothing reached the task manager")
		})
	}
}

// The status tells the CMS whether retrying is worth its while.
func TestHandleWebhook_AnswersSoRetriesStop(t *testing.T) {
	t.Run("an order raised somewhere else", func(t *testing.T) {
		h, tasks := handlerOver(t, false)
		rec := post(t, h, approvedBody, Sign([]byte(approvedBody), secret))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.False(t, tasks.called)
	})

	t.Run("an event from neither lifecycle", func(t *testing.T) {
		const body = `{"event": "vessel.departed", "slug": "x"}`
		h, tasks := handlerOver(t, true)
		rec := post(t, h, body, Sign([]byte(body), secret))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.False(t, tasks.called)
	})

	t.Run("a body that is not JSON", func(t *testing.T) {
		const body = `not json at all`
		h, _ := handlerOver(t, true)
		rec := post(t, h, body, Sign([]byte(body), secret))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// A deployment without the shared secret must not expose the route at all.
func TestNewHandler_RequiresASecret(t *testing.T) {
	_, err := NewHandler(&OrderEvents{}, &InvoiceEvents{}, Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signing secret is required")
}

// The same route carries both lifecycles, and the event name is what decides who
// reads the body — an invoice event must not be read as an order decision.
func TestHandleWebhook_RoutesByEventFamily(t *testing.T) {
	const invoiceBody = `{
		"event": "invoice.generated",
		"slug": "8d326f3a-643a-4a1d-8072-87130288b032",
		"invoice_no": "INV-2026-04412",
		"details": {"invoice_details": {"invoice_url": "https://slpacargoapi.slpa.lk/invoices/INV-2026-04412.pdf",
		                                "total_payable_lkr": 4820.5}}
	}`

	h, tasks := handlerOver(t, true)
	rec := post(t, h, invoiceBody, Sign([]byte(invoiceBody), secret))

	assert.Equal(t, http.StatusOK, rec.Code)
	require.True(t, tasks.called)
	// The invoice side's payload, not a decision: an invoice event carries no
	// "decision" at all.
	assert.NotContains(t, tasks.payload, "decision")
	assert.Equal(t, false, tasks.payload["paid"])
	assert.Equal(t, "https://slpacargoapi.slpa.lk/invoices/INV-2026-04412.pdf", tasks.payload["invoice_url"])
	assert.Equal(t, 4820.5, tasks.payload["payable"])
}
