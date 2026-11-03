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

	"github.com/OpenNSW/core/pagination"
	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// fakeCaseRepo holds cases and, per case, the OU that owns it and in which roles.
type fakeCaseRepo struct {
	cases  map[string]Case
	owners map[string]Owner // case id -> its owning OU and roles
}

func (f fakeCaseRepo) owns(o Owner, id string) bool {
	owner, ok := f.owners[id]
	return ok && owner.OUHandle == o.OUHandle && slices.ContainsFunc(o.Roles, func(r string) bool { return slices.Contains(owner.Roles, r) })
}

func (f fakeCaseRepo) ListCases(_ context.Context, o Owner, _, _ int) ([]Case, int64, error) {
	out := []Case{}
	for id, c := range f.cases {
		if f.owns(o, id) {
			out = append(out, c)
		}
	}
	return out, int64(len(out)), nil
}

func (f fakeCaseRepo) GetCase(_ context.Context, o Owner, id string) (*Case, error) {
	c, ok := f.cases[id]
	if !ok || !f.owns(o, id) {
		return nil, ErrCaseNotFound
	}
	return &c, nil
}

// fakeTasks answers GetTasks the way core's store does, through TaskFilter.Matches, and
// records each filter it was asked with.
type fakeTasks struct {
	tasks   []store.TaskRecord
	filters *[]store.TaskFilter
	// err, when set, is a failed read.
	err error
}

func (f fakeTasks) GetTasks(_ context.Context, filter store.TaskFilter) ([]store.TaskRecord, error) {
	if f.filters != nil {
		*f.filters = append(*f.filters, filter)
	}
	if f.err != nil {
		return nil, f.err
	}
	var out []store.TaskRecord
	for _, t := range f.tasks {
		if filter.Matches(t) {
			out = append(out, t)
		}
	}
	return out, nil
}

// officer is a CDA user holding roles.
func officer(roles ...string) *authn.Principal {
	return &authn.Principal{Kind: authn.KindUser, OUHandle: "cda", Roles: roles}
}

// requestBy builds a request authenticated as p, or unauthenticated when p is nil.
func requestBy(p *authn.Principal, target string, id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if id != "" {
		r.SetPathValue("id", id)
	}
	if p != nil {
		r = r.WithContext(authn.ContextWithPrincipal(r.Context(), p))
	}
	return r
}

// newCaseFixture serves case C1, owned by CDA's OU as officer, with tasks of several
// roles, and returns the filters its task store is asked with.
func newCaseFixture(t *testing.T) (*CaseHandler, *[]store.TaskFilter) {
	t.Helper()
	now := time.Now()
	repo := fakeCaseRepo{
		cases:  map[string]Case{"C1": {ID: "C1", State: "IN_PROGRESS", CreatedAt: now, UpdatedAt: now}},
		owners: map[string]Owner{"C1": {OUHandle: "cda", Roles: []string{"officer"}}},
	}
	officerRole := []string{"officer"}
	var filters []store.TaskFilter
	// Every flow on a case files its tasks under the case ID.
	tasks := fakeTasks{filters: &filters, tasks: []store.TaskRecord{
		{TaskID: "n1", CaseID: "C1", ReadRoles: officerRole, TaskType: "APPLICATION", State: "PENDING_USER", TaskTemplateID: "tpl-a"},
		{TaskID: "n2", CaseID: "C1", ReadRoles: officerRole, TaskType: "APPLICATION", State: "COMPLETED", TaskTemplateID: "tpl-b"},
		{TaskID: "n3", CaseID: "C1", ReadRoles: officerRole, TaskType: "SYSTEM", State: "COMPLETED"},
		{TaskID: "n4", CaseID: "C1", ReadRoles: []string{"reviewer"}, TaskType: "REVIEW", State: "PENDING_USER", TaskTemplateID: "tpl-r"},
		{TaskID: "n5", CaseID: "C1", TaskType: "APPLICATION", State: "PENDING_USER", TaskTemplateID: "tpl-none"},
		{TaskID: "other-case", CaseID: "C9", ReadRoles: officerRole, TaskType: "APPLICATION", State: "PENDING_USER"},
	}}
	return NewCaseHandler(repo, tasks, newSet(t, nil, nil), map[string]string{"officer": "Officer", "reviewer": "Reviewer"}), &filters
}

func TestCaseHandlerRequiresACatalogRole(t *testing.T) {
	h, _ := newCaseFixture(t)
	for name, tc := range map[string]struct {
		p    *authn.Principal
		want int
	}{
		"unauthenticated": {nil, http.StatusUnauthorized},
		"trader":          {officer("Trader"), http.StatusForbidden},
		// Only a user can be an officer, whatever roles a client token carries.
		"client":      {&authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA", Roles: []string{"Officer"}}, http.StatusForbidden},
		"officer":     {officer("Officer"), http.StatusOK},
		"mixed roles": {officer("Trader", "Officer"), http.StatusOK},
	} {
		rec := httptest.NewRecorder()
		h.HandleListCases(rec, requestBy(tc.p, "/api/v1/cases", ""))
		if rec.Code != tc.want {
			t.Errorf("list as %s: status = %d, want %d", name, rec.Code, tc.want)
		}
		rec = httptest.NewRecorder()
		h.HandleGetCase(rec, requestBy(tc.p, "/api/v1/cases/C1", "C1"))
		if rec.Code != tc.want {
			t.Errorf("detail as %s: status = %d, want %d", name, rec.Code, tc.want)
		}
	}
}

// A case is shown only to an OU that owns it, in a role the caller's token holds.
func TestCaseHandlerShowsOnlyTheCallersCases(t *testing.T) {
	h, _ := newCaseFixture(t)
	for name, p := range map[string]*authn.Principal{
		"another role": officer("Reviewer"),
		"another OU":   {Kind: authn.KindUser, OUHandle: "sltb", Roles: []string{"Officer"}},
	} {
		rec := httptest.NewRecorder()
		h.HandleListCases(rec, requestBy(p, "/api/v1/cases", ""))
		var page pagination.Page[CaseSummary]
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != http.StatusOK || len(page.Items) != 0 {
			t.Errorf("list as %s: status = %d, body %s, want no cases", name, rec.Code, rec.Body)
		}
		rec = httptest.NewRecorder()
		h.HandleGetCase(rec, requestBy(p, "/api/v1/cases/C1", "C1"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("detail as %s: status = %d, want 404", name, rec.Code)
		}
	}
}

// The detail lists the case's tasks the officer may read: those whose read.roles name a
// catalog role they hold, asking the store once for the case with those catalog roles.
func TestCaseHandlerDetailListsTheReadableTasks(t *testing.T) {
	h, filters := newCaseFixture(t)

	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, requestBy(officer("Reviewer", "Trader", "Officer"), "/api/v1/cases/C1", "C1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got CaseDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// n3 is SYSTEM, n5 lists no read roles, and other-case is another case's.
	var ids []string
	for _, n := range got.WorkflowNodes {
		ids = append(ids, n.ID)
	}
	if !slices.Equal(ids, []string{"n1", "n2", "n4"}) {
		t.Errorf("nodes = %v, want n1, n2 and n4", ids)
	}
	// No render config resolves, so each falls back to its template id.
	if n1 := got.WorkflowNodes[0]; n1.State != "IN_PROGRESS" || n1.WorkflowNodeTemplate.Name != "tpl-a" {
		t.Errorf("n1 = %+v", n1)
	}
	// The catalog roles, not the token roles; a token role outside the catalog adds none.
	want := []store.TaskFilter{{CaseID: "C1", Roles: []string{"officer", "reviewer"}}}
	if !reflect.DeepEqual(*filters, want) {
		t.Errorf("filters = %+v, want %+v", *filters, want)
	}
}

// Tasks that could not be read are a server error, not a case with no tasks.
func TestCaseHandlerDetailFailsWhenTheTasksCannotBeRead(t *testing.T) {
	h, _ := newCaseFixture(t)
	h.tasks = fakeTasks{err: errors.New("db down")}

	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, requestBy(officer("Officer"), "/api/v1/cases/C1", "C1"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestCaseHandlerUnknownCase(t *testing.T) {
	h, _ := newCaseFixture(t)
	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, requestBy(officer("Officer"), "/api/v1/cases/nope", "nope"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Catalog roles sharing a token role are all held by an officer holding it, so a case
// owned in either is theirs.
func TestCaseHandlerSharedTokenRole(t *testing.T) {
	var filters []store.TaskFilter
	repo := fakeCaseRepo{
		cases:  map[string]Case{"C1": {ID: "C1"}},
		owners: map[string]Owner{"C1": {OUHandle: "cda", Roles: []string{"accountant"}}},
	}
	h := NewCaseHandler(repo, fakeTasks{filters: &filters}, newSet(t, nil, nil),
		map[string]string{"accountant": "CDA Officer", "officer": "CDA Officer"})

	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, requestBy(officer("CDA Officer", "CDA Officer"), "/api/v1/cases/C1", "C1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := []store.TaskFilter{{CaseID: "C1", Roles: []string{"accountant", "officer"}}}
	if !reflect.DeepEqual(filters, want) {
		t.Errorf("filters = %+v, want %+v", filters, want)
	}
}

// A catalog entry mapped to "" must not match a caller whose roles hold an empty string.
func TestCaseHandlerEmptyTokenRole(t *testing.T) {
	h := NewCaseHandler(fakeCaseRepo{}, fakeTasks{}, newSet(t, nil, nil), map[string]string{"reviewer": ""})
	for _, roles := range [][]string{{""}, {"Reviewer"}, nil} {
		rec := httptest.NewRecorder()
		h.HandleListCases(rec, requestBy(officer(roles...), "/api/v1/cases", ""))
		if rec.Code != http.StatusForbidden {
			t.Errorf("roles %q: status = %d, want 403", roles, rec.Code)
		}
	}
}
