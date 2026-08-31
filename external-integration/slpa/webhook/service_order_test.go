package webhook

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const slug = "8d326f3a-643a-4a1d-8072-87130288b032"

// completer records what the service asked the task manager to do.
type orderCompleter struct {
	taskID  string
	payload map[string]any
	called  bool
}

func (c *orderCompleter) CompleteTaskStep(_ context.Context, taskID string, payload map[string]any) error {
	c.taskID, c.payload, c.called = taskID, payload, true
	return nil
}

func newOrderEvents(t *testing.T) (*OrderEvents, sqlmock.Sqlmock, *orderCompleter) {
	t.Helper()

	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	tasks := &orderCompleter{}
	return NewOrderEvents(db, tasks), mock, tasks
}

// expectParked answers the lookup for the task waiting on this order.
func expectParked(mock sqlmock.Sqlmock, taskID string) {
	mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).
		WithArgs(slug, ApprovalWaitTemplateID, stateQueuedExternally, 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow(taskID))
}

func orderEvent(name string) OrderEvent {
	return OrderEvent{
		Event:          name,
		Slug:           slug,
		ServiceOrderNo: "SO-FCL-EXPORT-2026-262316",
		CusdecSerial:   "BIBE1CBEX1-2026-E-10642026",
		Status:         "act_approve",
		InvoiceNo:      "INV-2026-04412",
		TotalAmount:    16,
		Timestamp:      "2026-08-26T02:30:59.846Z",
	}
}

// The accountant's approval is the end of SLPA's review, so the step waiting on
// it is released and the flow moves on.
func TestOrderEvents_ApprovalReleasesTheStep(t *testing.T) {
	service, mock, tasks := newOrderEvents(t)
	expectParked(mock, "slpa_3_0_decision:abc")

	require.NoError(t, service.Handle(context.Background(), orderEvent(EventApprovedByAccountant)))
	require.NoError(t, mock.ExpectationsWereMet())

	require.True(t, tasks.called)
	assert.Equal(t, "slpa_3_0_decision:abc", tasks.taskID)
	assert.Equal(t, "submit", tasks.payload["__command"])
	assert.Equal(t, DecisionApproved, tasks.payload["decision"])
	assert.Equal(t, true, tasks.payload["final"])
	assert.Equal(t, "Approved by the accountant", tasks.payload["stage"])
	assert.Equal(t, "INV-2026-04412", tasks.payload["invoice_no"])
	assert.Equal(t, float64(16), tasks.payload["total_amount"])
}

// Either refusal ends SLPA's review too: the trader has to act, so the step must
// not sit waiting for an approval that is not coming.
func TestOrderEvents_RejectionReleasesTheStep(t *testing.T) {
	for _, name := range []string{EventRejectedByAccountsClerk, EventRejectedByAccountant} {
		t.Run(name, func(t *testing.T) {
			service, mock, tasks := newOrderEvents(t)
			expectParked(mock, "task-1")

			require.NoError(t, service.Handle(context.Background(), orderEvent(name)))
			require.NoError(t, mock.ExpectationsWereMet())
			assert.Equal(t, DecisionRejected, tasks.payload["decision"])
		})
	}
}

// The clerk's approval, and a return to the clerk, leave the order with SLPA.
// The trader is shown where it is; the step stays open.
// The clerk's approval, and a return to the clerk, leave the order with SLPA.
// They still go through the task manager — nothing here edits a record in place —
// and final:false is what sends the workflow back to the same wait.
func TestOrderEvents_ProgressGoesThroughTheTaskManagerToo(t *testing.T) {
	for name, stage := range map[string]string{
		EventApprovedByAccountsClerk: "Approved by the accounts clerk, with the accountant",
		EventRejectedToAccountsClerk: "Returned by the accountant to the accounts clerk",
	} {
		t.Run(name, func(t *testing.T) {
			service, mock, tasks := newOrderEvents(t)
			expectParked(mock, "task-1")

			require.NoError(t, service.Handle(context.Background(), orderEvent(name)))
			require.NoError(t, mock.ExpectationsWereMet())

			require.True(t, tasks.called)
			assert.Equal(t, false, tasks.payload["final"], "the order is still with SLPA")
			assert.Equal(t, DecisionPending, tasks.payload["decision"])
			assert.Equal(t, stage, tasks.payload["stage"])
		})
	}
}

// A redelivery of a decision already applied finds nothing parked. The CMS is
// answered rather than told to retry, which is what stops the retries.
func TestOrderEvents_RedeliveryIsAnswered(t *testing.T) {
	service, mock, tasks := newOrderEvents(t)

	mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("COMPLETED"))

	require.NoError(t, service.Handle(context.Background(), orderEvent(EventApprovedByAccountant)))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.False(t, tasks.called)
}

// An order this deployment never raised is a different matter: the CMS is told
// so, and may retry in case the two are racing.
func TestOrderEvents_UnknownOrder(t *testing.T) {
	service, mock, _ := newOrderEvents(t)

	mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"state"}))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	assert.ErrorIs(t, service.Handle(context.Background(), orderEvent(EventApprovedByAccountant)), ErrOrderNotFound)
}

// The decision arrives before the task that waits for it — the consignment holds
// the order, but this wait has not opened yet. Their retry resolves it.
func TestOrderEvents_BeforeTheWaitOpens(t *testing.T) {
	service, mock, tasks := newOrderEvents(t)

	mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"state"}))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	assert.ErrorIs(t, service.Handle(context.Background(), orderEvent(EventApprovedByAccountant)), ErrNotWaitingYet)
	assert.False(t, tasks.called)
}

// A redelivered final approval must be answered even while the invoice task that
// followed it is open. Both tasks carry the same slug, so judging the redelivery
// by every row would let that sibling say "retry" for a decision already applied
// — and SLPA would keep retrying it for as long as the invoice went unpaid.
func TestOrderEvents_RedeliveryIsAnsweredWhileTheInvoiceTaskRuns(t *testing.T) {
	service, mock, tasks := newOrderEvents(t)

	mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).
		WillReturnError(gorm.ErrRecordNotFound)
	// Scoped to the approval wait: the invoice task's QUEUED_EXTERNALLY row
	// carries the same slug and must not be consulted here.
	mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).
		WithArgs(slug, ApprovalWaitTemplateID).
		WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("COMPLETED"))

	require.NoError(t, service.Handle(context.Background(), orderEvent(EventApprovedByAccountant)))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.False(t, tasks.called)
}

// Between steps the wait is neither parked nor finished: the previous event is
// still being applied. Answering "already applied" would drop an event nothing
// acted on, so the CMS is asked to retry.
func TestOrderEvents_BetweenStepsAsksForARetry(t *testing.T) {
	service, mock, tasks := newOrderEvents(t)

	mock.ExpectQuery(`SELECT .*FROM "task_records_v2"`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`SELECT "state" FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("STARTING"))

	assert.ErrorIs(t, service.Handle(context.Background(), orderEvent(EventApprovedByAccountant)), ErrNotWaitingYet)
	assert.False(t, tasks.called)
}

func TestOrderEvents_RefusesWhatItCannotActOn(t *testing.T) {
	service, _, _ := newOrderEvents(t)

	t.Run("an event that is not modelled", func(t *testing.T) {
		e := orderEvent("service_order.something_else")
		assert.ErrorIs(t, service.Handle(context.Background(), e), ErrUnknownEvent)
	})

	t.Run("no slug to correlate by", func(t *testing.T) {
		e := orderEvent(EventApprovedByAccountant)
		e.Slug = "  "
		err := service.Handle(context.Background(), e)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a slug")
	})
}

func TestOrderEventDecisions(t *testing.T) {
	for name, want := range map[string]string{
		EventApprovedByAccountant:    DecisionApproved,
		EventRejectedByAccountsClerk: DecisionRejected,
		EventRejectedByAccountant:    DecisionRejected,
		EventApprovedByAccountsClerk: DecisionPending,
		EventRejectedToAccountsClerk: DecisionPending,
		"service_order.invented":     "",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, want, OrderEvent{Event: name}.Decision())
		})
	}
}
