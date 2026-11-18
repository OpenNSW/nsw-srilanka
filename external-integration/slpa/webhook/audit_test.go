package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/OpenNSW/core/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// fakeOrders answers the audit's order lookup the way a test wants.
type fakeOrders struct {
	order   recordedOrder
	found   bool
	err     error
	lookups []string
}

func (f *fakeOrders) recordedOrder(_ context.Context, correlator string) (recordedOrder, bool, error) {
	f.lookups = append(f.lookups, correlator)
	return f.order, f.found, f.err
}

// recordedOrders answers every lookup with the order a consignment recorded.
func recordedOrders() *fakeOrders {
	return &fakeOrders{order: recordedOrder{Slug: slug, CaseID: "case-1"}, found: true}
}

// failingCompleter fails, or panics, when asked to complete a step.
type failingCompleter struct {
	err    error
	panics bool
}

func (c failingCompleter) CompleteTaskStep(context.Context, string, string, map[string]any) error {
	if c.panics {
		panic("task manager panicked")
	}
	return c.err
}

// audited returns h recording its deliveries, with orders answering its lookups.
func audited(h *Handler, orders *fakeOrders) (*Handler, *nswaudit.CaptureSink) {
	client, capture := nswaudit.NewWithCapture()
	h.WithAuditor(client)
	h.recorded = orders
	return h, capture
}

// postSigned posts body signed with the shared secret, in a request carrying a trace.
func postSigned(h *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/slpa", strings.NewReader(body))
	req = req.WithContext(trace.ContextWithTraceID(req.Context(), "slpa-trace"))
	req.Header.Set(SignatureHeader, Sign([]byte(body), secret))
	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, req)
	return rec
}

// paidBody is an invoice payment as the CMS sends it, with the receipt's private
// detail alongside what names the order.
const paidBody = `{
	"event": "invoice.paid",
	"slug": "8d326f3a-643a-4a1d-8072-87130288b032",
	"service_order_no": "SO-FCL-EXPORT-2026-262316",
	"invoice_no": "INV-2026-04412",
	"status": "paid",
	"payment_receipt": {
		"paid_amount": 39402,
		"payment_receipt": "100415624",
		"payment_receipt_url": "https://slpacargoapi.slpa.lk/pdf/payment-receipt/INV-2026-04412?signature=receipt-signature",
		"shipper": "Private Shipper Ltd",
		"consignee": "Private Consignee Pvt"
	}
}`

// marshaled is a record's metadata as the audit log line carries it.
func marshaled(t *testing.T, r nswaudit.Record) string {
	t.Helper()
	b, err := json.Marshal(r.Metadata)
	require.NoError(t, err)
	return string(b)
}

// An applied delivery is recorded once, against the order and case it names, with
// the CMS as the actor and nothing of the order's contents.
func TestHandleWebhook_AuditsAppliedDeliveries(t *testing.T) {
	for name, tc := range map[string]struct {
		body, event string
	}{
		"approval":        {approvedBody, EventApprovedByAccountant},
		"invoice payment": {paidBody, EventInvoicePaid},
	} {
		t.Run(name, func(t *testing.T) {
			orders := recordedOrders()
			h, capture := audited(handlerWith(t, true, &orderCompleter{}), orders)

			rec := postSigned(h, tc.body)
			require.Equal(t, http.StatusOK, rec.Code)

			records := capture.Records()
			require.Len(t, records, 1)
			r := records[0]
			assert.Equal(t, nswaudit.EventSLPA, r.EventType)
			assert.Equal(t, nswaudit.ActionUpdate, r.Action)
			assert.Equal(t, nswaudit.StatusSuccess, r.Status)
			assert.Equal(t, nswaudit.ActorService, r.ActorType)
			assert.Equal(t, "slpa", r.ActorID)
			assert.Equal(t, "slpa-trace", r.TraceID)
			assert.Equal(t, nswaudit.TargetServiceOrder, r.TargetType)
			assert.Equal(t, slug, r.TargetID)
			assert.Equal(t, map[string]any{
				"source":         "slpa",
				"httpStatus":     http.StatusOK,
				"eventType":      tc.event,
				"slug":           slug,
				"serviceOrderNo": "SO-FCL-EXPORT-2026-262316",
				"invoiceNo":      "INV-2026-04412",
				"caseId":         "case-1",
			}, r.Metadata)
			assert.Empty(t, r.Message)
			assert.Equal(t, []string{slug}, orders.lookups)

			metadata := marshaled(t, r)
			for _, private := range []string{"BIBE1CBEX1", "Private Shipper", "Private Consignee", "receipt-signature", "39402"} {
				assert.NotContains(t, metadata, private)
			}
		})
	}
}

// An invoice payment that names its order only by number is resolved by it.
func TestHandleWebhook_AuditResolvesAnOrderByItsNumber(t *testing.T) {
	const body = `{"event":"invoice.paid","service_order_no":" SO-FCL-EXPORT-2026-262316 ","invoice_no":"INV-2026-04412"}`
	orders := recordedOrders()
	h, capture := audited(handlerWith(t, true, &orderCompleter{}), orders)

	require.Equal(t, http.StatusOK, postSigned(h, body).Code)

	records := capture.Records()
	require.Len(t, records, 1)
	assert.Equal(t, slug, records[0].TargetID)
	assert.NotContains(t, records[0].Metadata, "slug")
	assert.Equal(t, []string{"SO-FCL-EXPORT-2026-262316"}, orders.lookups)
}

// A refused or failed delivery is recorded as a failure, with the status it was
// answered with and no detail of why.
func TestHandleWebhook_AuditsRefusedDeliveries(t *testing.T) {
	for name, tc := range map[string]struct {
		parked     bool
		tasks      TaskCompleter
		orders     *fakeOrders
		body       string
		wantHTTP   int
		wantTarget nswaudit.TargetType
		wantID     string
		wantEvent  any
		wantCode   string
		lookups    []string
	}{
		"an order raised somewhere else": {
			parked: false, tasks: &orderCompleter{}, orders: &fakeOrders{}, body: approvedBody,
			wantHTTP: http.StatusNotFound, wantTarget: nswaudit.TargetServiceOrder,
			wantEvent: EventApprovedByAccountant, wantCode: "callback_rejected", lookups: []string{slug},
		},
		"the task manager failing": {
			parked: true, tasks: failingCompleter{err: errors.New("private task manager error")}, orders: recordedOrders(), body: approvedBody,
			wantHTTP: http.StatusInternalServerError, wantTarget: nswaudit.TargetServiceOrder, wantID: slug,
			wantEvent: EventApprovedByAccountant, wantCode: "callback_rejected", lookups: []string{slug},
		},
		"an event this route does not apply": {
			parked: true, tasks: &orderCompleter{}, orders: recordedOrders(), body: `{"event":"vessel.departed","slug":"x"}`,
			wantHTTP: http.StatusBadRequest, wantTarget: nswaudit.TargetSLPACallback,
			wantEvent: "UNKNOWN", wantCode: "callback_rejected",
		},
		"a body that is not JSON": {
			parked: true, tasks: &orderCompleter{}, orders: recordedOrders(), body: `not json at all`,
			wantHTTP: http.StatusBadRequest, wantTarget: nswaudit.TargetSLPACallback,
			wantEvent: nil, wantCode: "invalid_callback_payload",
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, capture := audited(handlerWith(t, tc.parked, tc.tasks), tc.orders)

			require.Equal(t, tc.wantHTTP, postSigned(h, tc.body).Code)

			records := capture.Records()
			require.Len(t, records, 1)
			r := records[0]
			assert.Equal(t, nswaudit.StatusFailure, r.Status)
			assert.Equal(t, tc.wantTarget, r.TargetType)
			assert.Equal(t, tc.wantID, r.TargetID)
			assert.Equal(t, tc.wantHTTP, r.Metadata["httpStatus"])
			assert.Equal(t, tc.wantEvent, r.Metadata["eventType"])
			assert.Equal(t, tc.wantCode, r.Metadata["error_code"])
			assert.Equal(t, tc.lookups, tc.orders.lookups)

			metadata := marshaled(t, r)
			assert.NotContains(t, metadata, "vessel.departed")
			assert.NotContains(t, metadata, "private task manager error")
		})
	}
}

// A delivery that fails the signature check is left to the security log: its
// body cannot be trusted to name anything.
func TestHandleWebhook_DoesNotAuditUnauthenticatedDeliveries(t *testing.T) {
	orders := recordedOrders()
	h, capture := audited(handlerWith(t, true, &orderCompleter{}), orders)

	rec := post(t, h, approvedBody, Sign([]byte(approvedBody), "not-the-secret"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, capture.Records())
	assert.Empty(t, orders.lookups)
}

// An order that cannot be resolved for the record, missing or behind a failed
// lookup, changes nothing about the response.
func TestHandleWebhook_AuditLookupDoesNotChangeTheResponse(t *testing.T) {
	for name, orders := range map[string]*fakeOrders{
		"not recorded":  {},
		"lookup failed": {err: errors.New("database unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			h, capture := audited(handlerWith(t, true, &orderCompleter{}), orders)

			require.Equal(t, http.StatusOK, postSigned(h, approvedBody).Code)

			records := capture.Records()
			require.Len(t, records, 1)
			r := records[0]
			assert.Equal(t, nswaudit.StatusSuccess, r.Status)
			assert.Equal(t, nswaudit.TargetServiceOrder, r.TargetType)
			assert.Empty(t, r.TargetID)
			assert.NotContains(t, r.Metadata, "caseId")
			assert.NotEmpty(t, r.Metadata["targetResolutionError"])
			assert.Equal(t, slug, r.Metadata["slug"])
		})
	}
}

// A delivery whose processing panicked wrote no response, and is recorded as the
// failure the server answers it with.
func TestHandleWebhook_AuditsAPanicAsAFailure(t *testing.T) {
	h, capture := audited(handlerWith(t, true, failingCompleter{panics: true}), recordedOrders())

	assert.Panics(t, func() { postSigned(h, approvedBody) })

	records := capture.Records()
	require.Len(t, records, 1)
	assert.Equal(t, nswaudit.StatusFailure, records[0].Status)
	assert.Equal(t, http.StatusInternalServerError, records[0].Metadata["httpStatus"])
}

// An identifier over the bound is left out of the record, while the lookup still
// uses it as sent.
func TestHandleWebhook_AuditBoundsIdentifiers(t *testing.T) {
	for _, length := range []int{maxAuditIDLen, maxAuditIDLen + 1} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			long := strings.Repeat("s", length)
			body := fmt.Sprintf(`{"event":%q,"slug":%q,"service_order_no":%q,"invoice_no":%q}`,
				EventApprovedByAccountant, long, long, long)
			orders := recordedOrders()
			h, capture := audited(handlerWith(t, true, &orderCompleter{}), orders)

			require.Equal(t, http.StatusOK, postSigned(h, body).Code)

			records := capture.Records()
			require.Len(t, records, 1)
			for _, key := range []string{"slug", "serviceOrderNo", "invoiceNo"} {
				if length <= maxAuditIDLen {
					assert.Equal(t, long, records[0].Metadata[key])
				} else {
					assert.NotContains(t, records[0].Metadata, key)
				}
			}
			assert.Equal(t, []string{long}, orders.lookups)
		})
	}
}

// Without an auditor the handler records nothing and looks nothing up.
func TestHandleWebhook_AuditIsOptional(t *testing.T) {
	h, tasks := handlerOver(t, true)
	require.Nil(t, h.recorded)

	require.Equal(t, http.StatusOK, postSigned(h, approvedBody).Code)
	assert.True(t, tasks.called)
}

// The recorded order is read from the tasks that hold it, by its slug or number.
func TestTaskLookup_RecordedOrder(t *testing.T) {
	const query = `SELECT data->'so'->>'slug' AS slug,"case_id" FROM "taskflow_tasks" ` +
		`WHERE data->'so'->>'slug' = $1 OR data->'so'->>'service_order_no' = $2 LIMIT $3`

	for name, tc := range map[string]struct {
		rows    *sqlmock.Rows
		err     error
		want    recordedOrder
		found   bool
		wantErr bool
	}{
		"recorded":      {rows: sqlmock.NewRows([]string{"slug", "case_id"}).AddRow(slug, "case-1"), want: recordedOrder{Slug: slug, CaseID: "case-1"}, found: true},
		"not recorded":  {rows: sqlmock.NewRows([]string{"slug", "case_id"})},
		"lookup failed": {err: errors.New("database unavailable"), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			conn, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{
				Logger: logger.Default.LogMode(logger.Silent),
			})
			require.NoError(t, err)

			expect := mock.ExpectQuery(query).WithArgs("SO-1", "SO-1", 1)
			if tc.err != nil {
				expect.WillReturnError(tc.err)
			} else {
				expect.WillReturnRows(tc.rows)
			}

			order, found, err := taskLookup{db: db}.recordedOrder(context.Background(), "SO-1")
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, order)
			assert.Equal(t, tc.found, found)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The recorded status is the first one written, or the 200 a bare write implies,
// and the response passes through unchanged.
func TestRecordStatus(t *testing.T) {
	for name, tc := range map[string]struct {
		write func(http.ResponseWriter)
		want  int
	}{
		"explicit":    {func(w http.ResponseWriter) { w.WriteHeader(http.StatusConflict) }, http.StatusConflict},
		"first wins":  {func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound); w.WriteHeader(http.StatusOK) }, http.StatusNotFound},
		"bare write":  {func(w http.ResponseWriter) { _, _ = w.Write([]byte("ok")) }, http.StatusOK},
		"nothing yet": {func(http.ResponseWriter) {}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var status int
			tc.write(recordStatus(rec, &status))
			assert.Equal(t, tc.want, status)
			if tc.want != 0 {
				assert.Equal(t, tc.want, rec.Code)
			}
		})
	}
}
