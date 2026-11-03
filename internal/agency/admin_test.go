package agency

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/OpenNSW/core/taskflow/store"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
)

// fakeFlows lists each case's flows, or fails with err.
type fakeFlows struct {
	flows map[string][]caseflow.Flow
	err   error
}

func (f fakeFlows) Flows(_ context.Context, caseID string) ([]caseflow.Flow, error) {
	return f.flows[caseID], f.err
}

// newAdminFixture serves case C1, owned by CDA's OU as officer, with officer and reviewer
// tasks, one task with no read roles and a system task, and one flow; and returns the filters
// its task store is asked with and what it audits.
func newAdminFixture(t *testing.T, flows fakeFlows) (*AdminCaseHandler, *[]store.TaskFilter, func() []nswaudit.Record) {
	t.Helper()
	now := time.Now()
	repo := fakeCaseRepo{
		cases:  map[string]Case{"C1": {ID: "C1", State: "IN_PROGRESS", CreatedAt: now, UpdatedAt: now}},
		owners: map[string]Owner{"C1": {OUHandle: "cda", Roles: []string{"officer"}}},
	}
	var filters []store.TaskFilter
	tasks := fakeTasks{filters: &filters, tasks: []store.TaskRecord{
		{TaskID: "n1", CaseID: "C1", ReadRoles: []string{"officer"}, TaskType: "APPLICATION", State: "PENDING_USER", TaskTemplateID: "tpl-a"},
		{TaskID: "n2", CaseID: "C1", ReadRoles: []string{"reviewer"}, TaskType: "REVIEW", State: "COMPLETED", TaskTemplateID: "tpl-r"},
		{TaskID: "n3", CaseID: "C1", TaskType: "APPLICATION", State: "PENDING_USER", TaskTemplateID: "tpl-none"},
		{TaskID: "n4", CaseID: "C1", ReadRoles: []string{"officer"}, TaskType: "SYSTEM", State: "COMPLETED"},
		{TaskID: "other-case", CaseID: "C9", ReadRoles: []string{"officer"}, TaskType: "APPLICATION", State: "PENDING_USER"},
	}}
	client, capture := nswaudit.NewWithCapture()
	return NewAdminCaseHandler(repo, tasks, flows, newSet(t, nil, nil), client), &filters, capture.Records
}

func adminRequest(p *authn.Principal, id string) *http.Request {
	return requestBy(p, "/api/v1/admin/cases/"+id, id)
}

var admin = &authn.Principal{Kind: authn.KindUser, UserID: "admin-1", OUHandle: "nsw", Roles: []string{"CDA Admin"}}

// An admin reads any case, whoever owns it, with every task on it and its flows.
func TestAdminCaseHandler_ReadsAnyCase(t *testing.T) {
	started := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	h, filters, audited := newAdminFixture(t, fakeFlows{flows: map[string][]caseflow.Flow{"C1": {
		{WorkflowID: "task-1", CaseID: "C1", TemplateID: "cda_officer_verification", Status: caseflow.StatusStarted, CreatedAt: started, UpdatedAt: started},
	}}})

	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, adminRequest(admin, "C1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got AdminCaseDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// Every task of the case, whatever its read roles; not SYSTEM, nor another case's.
	var ids []string
	for _, n := range got.WorkflowNodes {
		ids = append(ids, n.ID)
	}
	if !slices.Equal(ids, []string{"n1", "n2", "n3"}) {
		t.Errorf("nodes = %v, want n1, n2 and n3", ids)
	}
	if want := []store.TaskFilter{{CaseID: "C1"}}; !reflect.DeepEqual(*filters, want) {
		t.Errorf("filters = %+v, want %+v", *filters, want)
	}
	// The flow, by workflow ID, named by its template when the manifest has no definition.
	want := []caseflow.View{{WorkflowID: "task-1", TemplateID: "cda_officer_verification", TemplateName: "cda_officer_verification",
		Status: "STARTED", CreatedAt: started.Format(time.RFC3339), UpdatedAt: started.Format(time.RFC3339)}}
	if !reflect.DeepEqual(got.Flows, want) {
		t.Errorf("flows = %+v, want %+v", got.Flows, want)
	}
	if recs := audited(); len(recs) != 1 || recs[0].TargetID != "C1" || recs[0].Metadata["view"] != "admin" {
		t.Errorf("audit = %+v, want one admin read of C1", recs)
	}
}

func TestAdminCaseHandler_Errors(t *testing.T) {
	for name, tc := range map[string]struct {
		p     *authn.Principal
		id    string
		flows fakeFlows
		want  int
	}{
		"unauthenticated":    {nil, "C1", fakeFlows{}, http.StatusUnauthorized},
		"a client":           {&authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA"}, "C1", fakeFlows{}, http.StatusUnauthorized},
		"unknown case":       {admin, "nope", fakeFlows{}, http.StatusNotFound},
		"flows fail to list": {admin, "C1", fakeFlows{err: errors.New("db down")}, http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			h, _, _ := newAdminFixture(t, tc.flows)
			rec := httptest.NewRecorder()
			h.HandleGetCase(rec, adminRequest(tc.p, tc.id))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
