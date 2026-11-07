package workflowadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: mockDB, DriverName: "postgres"}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func emptyArtifactSet(t *testing.T) *artifact.Set {
	t.Helper()
	// An empty, but present, manifest: render config and workflow definition lookups fall
	// back to the task template id / template id, which is all these tests need.
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"manifest_v1.json": []byte(`{"artifacts":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// fakeFlows lists each case's flows, or fails with err.
type fakeFlows struct {
	flows map[string][]caseflow.Flow
	err   error
}

func (f fakeFlows) Flows(_ context.Context, caseID string) ([]caseflow.Flow, error) {
	return f.flows[caseID], f.err
}

const caseRowColumns = "id, name, state, created_at, updated_at"

func caseRowQuery() string { return `SELECT ` + caseRowColumns + ` FROM cases WHERE id = \$1` }

// A case addresses whoever's id it is, whether it's a consignment's or an agency's own —
// every task on it whatever its read roles, and its flows.
func TestCaseService_GetCaseDetail(t *testing.T) {
	db, sqlMock := newMockDB(t)
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	sqlMock.ExpectQuery(caseRowQuery()).
		WithArgs("C1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}).
			AddRow("C1", "Shipment 1", "IN_PROGRESS", now, now))

	taskStore := &MockTaskStore{}
	tasks := []store.TaskRecord{
		{TaskID: "n1", CaseID: "C1", ReadRoles: []string{"officer"}, TaskType: "APPLICATION", State: "PENDING_USER", TaskTemplateID: "tpl-a", CreatedAt: now, UpdatedAt: now},
		{TaskID: "n2", CaseID: "C1", TaskType: "SYSTEM", State: "COMPLETED", CreatedAt: now, UpdatedAt: now},
	}
	// GetTasks is asked with only a CaseID: every task of the case is listed whatever its
	// read roles, since this is an admin view.
	taskStore.On("GetTasks", context.Background(), store.TaskFilter{CaseID: "C1"}).Return(tasks)

	flows := fakeFlows{flows: map[string][]caseflow.Flow{"C1": {
		{WorkflowID: "wf-1", CaseID: "C1", TemplateID: "trade-export", Status: caseflow.StatusStarted, CreatedAt: now, UpdatedAt: now},
	}}}
	svc := NewCaseService(db, taskStore, flows, emptyArtifactSet(t))

	detail, err := svc.GetCaseDetail(context.Background(), "C1")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if detail.CaseRow != (CaseRow{ID: "C1", Name: "Shipment 1", State: "IN_PROGRESS", CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}) {
		t.Errorf("case = %+v", detail.CaseRow)
	}
	// SYSTEM filtered out; the remaining task is listed whatever its read roles.
	if len(detail.WorkflowNodes) != 1 || detail.WorkflowNodes[0].ID != "n1" {
		t.Errorf("nodes = %+v, want just n1", detail.WorkflowNodes)
	}
	if detail.WorkflowNodes[0].WorkflowNodeTemplate.Name != "tpl-a" {
		t.Errorf("node name = %q, want the template id fallback", detail.WorkflowNodes[0].WorkflowNodeTemplate.Name)
	}
	want := []caseflow.View{{WorkflowID: "wf-1", TemplateID: "trade-export", TemplateName: "trade-export",
		Status: "STARTED", CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}}
	if !reflect.DeepEqual(detail.Flows, want) {
		t.Errorf("flows = %+v, want %+v", detail.Flows, want)
	}
}

func TestCaseService_GetCaseDetail_NotFound(t *testing.T) {
	db, sqlMock := newMockDB(t)
	sqlMock.ExpectQuery(caseRowQuery()).
		WithArgs("nope").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}))

	svc := NewCaseService(db, &MockTaskStore{}, fakeFlows{}, emptyArtifactSet(t))
	if _, err := svc.GetCaseDetail(context.Background(), "nope"); !errors.Is(err, ErrCaseNotFound) {
		t.Errorf("err = %v, want ErrCaseNotFound", err)
	}
}

func TestCaseService_GetCaseDetail_FlowsError(t *testing.T) {
	db, sqlMock := newMockDB(t)
	now := time.Now()
	sqlMock.ExpectQuery(caseRowQuery()).
		WithArgs("C1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}).
			AddRow("C1", nil, "IN_PROGRESS", now, now))

	taskStore := &MockTaskStore{}
	taskStore.On("GetTasks", context.Background(), store.TaskFilter{CaseID: "C1"}).Return([]store.TaskRecord(nil))
	svc := NewCaseService(db, taskStore, fakeFlows{err: errors.New("db down")}, emptyArtifactSet(t))

	if _, err := svc.GetCaseDetail(context.Background(), "C1"); err == nil {
		t.Error("err = nil, want the flows error")
	}
}

// Tasks that could not be read fail the detail rather than show a case with none.
func TestCaseService_GetCaseDetail_TasksError(t *testing.T) {
	db, sqlMock := newMockDB(t)
	now := time.Now()
	sqlMock.ExpectQuery(caseRowQuery()).
		WithArgs("C1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}).
			AddRow("C1", nil, "IN_PROGRESS", now, now))

	taskStore := &MockTaskStore{}
	taskStore.On("GetTasks", context.Background(), store.TaskFilter{CaseID: "C1"}).Return(nil, errors.New("db down"))
	svc := NewCaseService(db, taskStore, fakeFlows{}, emptyArtifactSet(t))

	if _, err := svc.GetCaseDetail(context.Background(), "C1"); err == nil || err.Error() != "list tasks of case C1: db down" {
		t.Errorf("err = %v, want the task store error", err)
	}
}

func adminCaseRequest(p *authn.Principal, id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cases/"+id, nil)
	r.SetPathValue("id", id)
	if p != nil {
		r = r.WithContext(authn.ContextWithPrincipal(r.Context(), p))
	}
	return r
}

var adminUser = &authn.Principal{Kind: authn.KindUser, UserID: "admin-1", OUHandle: "nsw", Roles: []string{"NSW Admin"}}

// The admin case route serves whatever CaseService returns, with no ownership check.
func TestHandler_HandleGetCase(t *testing.T) {
	db, sqlMock := newMockDB(t)
	now := time.Now()
	sqlMock.ExpectQuery(caseRowQuery()).
		WithArgs("C1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}).
			AddRow("C1", nil, "IN_PROGRESS", now, now))
	taskStore := &MockTaskStore{}
	// The request context carries the auth principal, unlike the CaseService-level tests above.
	taskStore.On("GetTasks", mock.Anything, store.TaskFilter{CaseID: "C1"}).Return([]store.TaskRecord(nil))
	cases := NewCaseService(db, taskStore, fakeFlows{}, emptyArtifactSet(t))
	client, capture := nswaudit.NewWithCapture()
	h := NewHandler(&Service{}, cases, client, testMaxRequestBytes)

	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, adminCaseRequest(adminUser, "C1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got CaseDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "C1" {
		t.Errorf("id = %q, want C1", got.ID)
	}
	if recs := capture.Records(); len(recs) != 1 || recs[0].TargetID != "C1" || recs[0].Metadata["view"] != "admin-case" {
		t.Errorf("audit = %+v, want one admin-case read of C1", recs)
	}
}

// Unauthenticated is rejected before any query; an unknown id is a 404.
func TestHandler_HandleGetCase_Unauthenticated(t *testing.T) {
	db, _ := newMockDB(t)
	cases := NewCaseService(db, &MockTaskStore{}, fakeFlows{}, emptyArtifactSet(t))
	h := NewHandler(&Service{}, cases, nil, testMaxRequestBytes)

	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, adminCaseRequest(nil, "C1"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandler_HandleGetCase_UnknownCase(t *testing.T) {
	db, sqlMock := newMockDB(t)
	sqlMock.ExpectQuery(caseRowQuery()).WithArgs("nope").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}))
	cases := NewCaseService(db, &MockTaskStore{}, fakeFlows{}, emptyArtifactSet(t))
	h := NewHandler(&Service{}, cases, nil, testMaxRequestBytes)

	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, adminCaseRequest(adminUser, "nope"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
