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

// SLPA's live payment, as they send it: flat, with the receipt on the envelope.
// Note total_amount 16 beside a paid_amount of 4776 — the order priced in
// dollars next to the rupees that actually moved — and payment_receipt carrying
// a reference number rather than a link.
const paid = `{
	"slug": "8d326f3a-643a-4a1d-8072-87130288b032",
	"event": "invoice.paid",
	"status": "client_paid_invoice",
	"timestamp": "2026-09-11T15:43:07+05:30",
	"invoice_no": "26211843261345",
	"total_amount": 16,
	"cusdec_serial": "BIBE1CBEX1-2026-E-32978892026",
	"payment_receipt": {
		"bl_no": "",
		"shipper": "JOTHI COCONUT EXPORTERS",
		"draft_id": "DRFT-INV-FCL-EXPORT-2026-257603",
		"consignee": "TSNW Test user",
		"cusdec_no": "BIBE1CBEX1-2026-E-32978892026",
		"invoice_no": "26211843261345",
		"paid_amount": 4776,
		"invoice_type": "export",
		"paid_datetime": "2026-09-11T15:43:07+05:30",
		"invoice_serial": "26SEP_LD1_00000017",
		"payment_receipt": "100415624",
		"service_order_no": "SO-FCL-EXPORT-2026-262342",
		"consignee_address": "test",
		"draft_requested_date": null,
		"service_order_requested_date": null
	},
	"service_order_id": 262342,
	"service_order_no": "SO-FCL-EXPORT-2026-262342"
}`

// An invoice.generated announcement is refused. The trader asks for the invoice
// from the step before this one, so the announcement carries nothing this side
// does not already hold, and the CMS should hear that its redelivery is
// pointless rather than retrying an event nothing acts on.
func TestInvoiceEvents_RefusesAnythingButThePayment(t *testing.T) {
	for _, event := range []string{"invoice.generated", "invoice.cancelled"} {
		t.Run(event, func(t *testing.T) {
			service, _, tasks := newInvoiceEvents(t)

			err := service.Handle(context.Background(), invoiceEvent(t, `{"event":"`+event+`","slug":"8d326f3a-643a-4a1d-8072-87130288b032"}`))

			require.ErrorIs(t, err, ErrUnknownEvent)
			assert.False(t, tasks.called, "nothing reached the task manager")
		})
	}
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
	assert.Equal(t, "26211843261345", tasks.payload["invoice_no"])
	assert.Equal(t, "SO-FCL-EXPORT-2026-262342", tasks.payload["service_order_no"])
	assert.Equal(t, "2026-09-11T15:43:07+05:30", tasks.payload["paid_at"])
	assert.Equal(t, "client_paid_invoice", tasks.payload["cms_status"])

	// The rupees that moved, not the dollars the order was priced in. The same
	// payload carries total_amount 16, which a settled panel must never show.
	assert.Equal(t, 4776.0, tasks.payload["payable_lkr"])

	// The receipt is a reference, not a document: SLPA sends no link on this
	// event, so nothing is offered as one.
	assert.Equal(t, "100415624", tasks.payload["receipt_no"])
	assert.NotContains(t, tasks.payload, "receipt_url")

	// Restated by the payment, so the settled panel keeps them.
	assert.Equal(t, "26SEP_LD1_00000017", tasks.payload["invoice_serial"])
}

// The envelope's total_amount is the order in dollars. Read as what was paid it
// would put "LKR 16.00" under a receipt for 4,776 — so it is not read at all.
func TestInvoiceEvent_NeverReportsTheDollarFigureAsPaid(t *testing.T) {
	assert.Equal(t, 4776.0, invoiceEvent(t, paid).Payable())

	nothingInRupees := invoiceEvent(t, `{"event":"invoice.paid","slug":"s","total_amount":16}`)
	assert.Equal(t, 0.0, nothingInRupees.Payable(),
		"an amount only in dollars is no amount at all")
}

// payment_receipt is "100415624". Behind a Download link it would go nowhere.
func TestInvoiceEvent_DoesNotOfferAReferenceNumberAsADocument(t *testing.T) {
	e := invoiceEvent(t, paid)
	assert.Equal(t, "100415624", e.ReceiptNo())
	assert.Empty(t, e.ReceiptURL())
}

// The older contract nested the same facts under details.invoice_details. A
// deployment still sending that shape must settle the step just as completely.
func TestInvoiceEvent_ReadsTheOlderNestedShapeToo(t *testing.T) {
	e := invoiceEvent(t, `{
		"event": "invoice.paid",
		"slug": "s",
		"details": {"invoice_details": {
			"invoice_serial": "BIBE1/2026/04412",
			"total_payable_lkr": 4820.5,
			"payment_slip_url": "https://slpacargoapi.slpa.lk/receipts/INV.pdf",
			"invoice_paid_at": "2026-08-27T09:15:00+05:30",
			"payment_receipt": {"payment_receipt": "RCPT-88213", "paid_amount": 4820.5}
		}}
	}`)

	assert.Equal(t, 4820.5, e.Payable())
	assert.Equal(t, "BIBE1/2026/04412", e.Serial())
	assert.Equal(t, "RCPT-88213", e.ReceiptNo())
	assert.Equal(t, "https://slpacargoapi.slpa.lk/receipts/INV.pdf", e.ReceiptURL())
	assert.Equal(t, "2026-08-27T09:15:00+05:30", e.PaidAt())
}

// A link that is there must be found; a value that is not a link must not be
// offered as one.
func TestInvoiceEvent_OffersOnlyWhatIsActuallyALink(t *testing.T) {
	t.Run("the invoice on the envelope", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.paid","slug":"s","invoice_url":"https://slpa/flat.pdf"}`)
		assert.Equal(t, "https://slpa/flat.pdf", e.InvoiceURL())
	})

	t.Run("the payment slip inside the order", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.paid","slug":"s","details":{"invoice_details":{"payment_slip_url":"https://slpa/slip.pdf"}}}`)
		assert.Equal(t, "https://slpa/slip.pdf", e.ReceiptURL())
	})

	// A paid invoice is stamped as paid, so it stands in when no slip is sent.
	t.Run("the invoice as the receipt of last resort", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.paid","slug":"s","details":{"invoice_details":{"invoice_url":"https://slpa/inv.pdf"}}}`)
		assert.Equal(t, "https://slpa/inv.pdf", e.ReceiptURL())
	})

	t.Run("a reference number is not a link", func(t *testing.T) {
		e := invoiceEvent(t, `{"event":"invoice.paid","slug":"s","details":{"invoice_details":{"invoice_url":"100415624"}}}`)
		assert.Empty(t, e.InvoiceURL())
		assert.Empty(t, e.ReceiptURL())
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
