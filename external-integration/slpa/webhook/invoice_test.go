package webhook

import (
	"context"
	"encoding/json"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type invoiceCompleter struct {
	taskID  string
	payload map[string]any
	called  bool
}

func (c *invoiceCompleter) CompleteTaskStep(_ context.Context, taskID string, payload map[string]any) error {
	c.taskID, c.payload, c.called = taskID, payload, true
	return nil
}

func newInvoiceEvents(t *testing.T) (*InvoiceEvents, sqlmock.Sqlmock, *invoiceCompleter) {
	t.Helper()

	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	tasks := &invoiceCompleter{}
	return NewInvoiceEvents(db, tasks), mock, tasks
}

func expectParkedInvoice(mock sqlmock.Sqlmock, taskID string) {
	mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow(taskID))
}

// event decodes a CMS payload, so the fixtures read as what they send.
func invoiceEvent(t *testing.T, payload string) InvoiceEvent {
	t.Helper()

	var e InvoiceEvent
	require.NoError(t, json.Unmarshal([]byte(payload), &e))
	return e
}

const generated = `{
	"event": "invoice.generated",
	"slug": "8d326f3a-643a-4a1d-8072-87130288b032",
	"service_order_no": "SO-FCL-EXPORT-2026-262318",
	"invoice_no": "INV-2026-04412",
	"details": {
		"invoice_details": {
			"invoice_no": "INV-2026-04412",
			"invoice_serial": "BIBE1/2026/04412",
			"status": "unpaid",
			"total_usd": 16,
			"total_lkr": 4800,
			"total_payable_lkr": 4820.5,
			"invoice_url": "https://slpacargoapi.slpa.lk/invoices/INV-2026-04412.pdf",
			"invoice_generated_at": "2026-08-26T11:02:00+05:30"
		}
	},
	"timestamp": "2026-08-26T11:02:00+05:30"
}`

const paid = `{
	"event": "invoice.paid",
	"slug": "8d326f3a-643a-4a1d-8072-87130288b032",
	"service_order_no": "SO-FCL-EXPORT-2026-262318",
	"invoice_no": "INV-2026-04412",
	"details": {
		"invoice_details": {
			"status": "paid",
			"total_payable_lkr": 4820.5,
			"invoice_url": "https://slpacargoapi.slpa.lk/invoices/INV-2026-04412.pdf",
			"payment_slip_url": "https://slpacargoapi.slpa.lk/receipts/INV-2026-04412.pdf",
			"invoice_paid_at": "2026-08-27T09:15:00+05:30",
			"payment_receipt": {"payment_receipt": "RCPT-88213", "paid_amount": 4820.5, "paid_datetime": "2026-08-27T09:15:00+05:30"}
		}
	},
	"timestamp": "2026-08-27T09:15:01+05:30"
}`

// The invoice being raised is not the end of anything: the trader still has to
// pay it, so what they owe and where to download it is recorded and the step
// stays open.
// The invoice being raised is not the end of anything: the trader still has to
// pay it. The step is resumed so what they owe is recorded through the task
// manager, and paid:false is what sends the workflow back to the same wait.
func TestInvoiceEvents_GeneratedRecordsTheInvoiceAndWaits(t *testing.T) {
	service, mock, tasks := newInvoiceEvents(t)
	expectParkedInvoice(mock, "slpa_4_0_invoice:abc")

	require.NoError(t, service.Handle(context.Background(), invoiceEvent(t, generated)))
	require.NoError(t, mock.ExpectationsWereMet())

	require.True(t, tasks.called, "recorded through the task manager, not written behind it")
	assert.Equal(t, false, tasks.payload["paid"], "the money has not moved yet")
	assert.Equal(t, "INV-2026-04412", tasks.payload["invoice_no"])
	assert.Equal(t, "https://slpacargoapi.slpa.lk/invoices/INV-2026-04412.pdf", tasks.payload["invoice_url"])
	assert.Equal(t, 4820.5, tasks.payload["payable"])
}

// The payment is what ends the step, and the receipt is what the trader keeps.
func TestInvoiceEvents_PaidReleasesTheStep(t *testing.T) {
	service, mock, tasks := newInvoiceEvents(t)
	expectParkedInvoice(mock, "slpa_4_0_invoice:abc")

	require.NoError(t, service.Handle(context.Background(), invoiceEvent(t, paid)))
	require.NoError(t, mock.ExpectationsWereMet())

	require.True(t, tasks.called)
	assert.Equal(t, "slpa_4_0_invoice:abc", tasks.taskID)
	assert.Equal(t, "submit", tasks.payload["__command"])
	assert.Equal(t, true, tasks.payload["paid"])
	assert.Equal(t, "INV-2026-04412", tasks.payload["invoice_no"])
	assert.Equal(t, "https://slpacargoapi.slpa.lk/receipts/INV-2026-04412.pdf", tasks.payload["receipt_url"])
	assert.Equal(t, "2026-08-27T09:15:00+05:30", tasks.payload["paid_at"])
	assert.Equal(t, 4820.5, tasks.payload["payable"])
}

// The documents are the point of these events, and the CMS puts them in more
// than one place — a link that is there must be found.
func TestInvoiceEvent_ReadsTheDocumentsFromWhereverTheyAre(t *testing.T) {
	t.Run("the invoice on the envelope", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.generated","slug":"s","invoice_url":"https://slpa/flat.pdf"}`)
		assert.Equal(t, "https://slpa/flat.pdf", e.InvoiceURL())
	})

	t.Run("the invoice inside the order", func(t *testing.T) {
		assert.Equal(t, "https://slpacargoapi.slpa.lk/invoices/INV-2026-04412.pdf", invoiceEvent(t, generated).InvoiceURL())
	})

	t.Run("the receipt, when only the payment receipt carries it", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.paid","slug":"s","details":{"invoice_details":{"payment_receipt":{"payment_receipt":"https://slpa/rcpt.pdf"}}}}`)
		assert.Equal(t, "https://slpa/rcpt.pdf", e.ReceiptURL())
	})

	// A paid invoice is stamped as paid, so it stands in when no receipt is sent.
	t.Run("the invoice as the receipt of last resort", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.paid","slug":"s","details":{"invoice_details":{"invoice_url":"https://slpa/inv.pdf"}}}`)
		assert.Equal(t, "https://slpa/inv.pdf", e.ReceiptURL())
	})

	t.Run("what is payable, preferring their own figure", func(t *testing.T) {
		assert.Equal(t, 4820.5, invoiceEvent(t, generated).Payable())

		usdOnly := invoiceEvent(t, `{"event":"invoice.generated","slug":"s","details":{"invoice_details":{"total_usd":16}}}`)
		assert.Equal(t, float64(16), usdOnly.Payable())
	})
}

// The slug is the correlator, but an event that carries only the order number is
// still tied to the right consignment.
func TestInvoiceEvents_MatchesOnTheOrderNumberWhenThereIsNoSlug(t *testing.T) {
	service, mock, tasks := newInvoiceEvents(t)
	expectParkedInvoice(mock, "task-1")

	e := invoiceEvent(t, paid)
	e.Slug = ""
	require.NoError(t, service.Handle(context.Background(), e))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, tasks.called)
}

func TestInvoiceEvents_RefusesWhatItCannotActOn(t *testing.T) {
	service, _, _ := newInvoiceEvents(t)

	t.Run("an event from neither lifecycle", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.cancelled","slug":"s"}`)
		assert.ErrorIs(t, service.Handle(context.Background(), e), ErrUnknownEvent)
	})

	t.Run("nothing to correlate on", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.paid"}`)
		err := service.Handle(context.Background(), e)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a slug or a service order number")
	})
}

// A redelivery finds nothing parked and is answered, which is what stops the
// CMS retrying; an invoice for an order raised elsewhere is reported as missing.
func TestInvoiceEvents_RedeliveryAndUnknownOrder(t *testing.T) {
	t.Run("redelivery", func(t *testing.T) {
		service, mock, tasks := newInvoiceEvents(t)
		mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).WillReturnError(gorm.ErrRecordNotFound)
		mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("COMPLETED"))

		require.NoError(t, service.Handle(context.Background(), invoiceEvent(t, paid)))
		assert.False(t, tasks.called)
	})

	t.Run("an order this deployment never raised", func(t *testing.T) {
		service, mock, _ := newInvoiceEvents(t)
		mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).WillReturnError(gorm.ErrRecordNotFound)
		mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).WillReturnRows(sqlmock.NewRows([]string{"state"}))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "task_records_v2"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		assert.ErrorIs(t, service.Handle(context.Background(), invoiceEvent(t, paid)), ErrOrderNotFound)
	})

	// The approval task carries the same slug, so an invoice event redelivered
	// after the flow finished must be judged by the invoice wait alone.
	t.Run("a redelivery once the invoice wait has finished", func(t *testing.T) {
		service, mock, tasks := newInvoiceEvents(t)
		mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).WillReturnError(gorm.ErrRecordNotFound)
		mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).
			WithArgs(slug, slug, PaymentWaitTemplateID).
			WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("COMPLETED"))

		require.NoError(t, service.Handle(context.Background(), invoiceEvent(t, paid)))
		require.NoError(t, mock.ExpectationsWereMet())
		assert.False(t, tasks.called)
	})
}
