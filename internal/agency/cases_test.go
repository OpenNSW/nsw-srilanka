package agency

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

type fakeTasks map[string][]store.TaskRecord // case id -> tasks

func (f fakeTasks) GetAllTasks(_ context.Context, caseID string) []store.TaskRecord {
	return f[caseID]
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

// newCaseHandler serves case C1, owned by CDA's OU as officer, with two officer tasks and
// a system one.
func newCaseHandler(t *testing.T) *CaseHandler {
	t.Helper()
	now := time.Now()
	repo := fakeCaseRepo{
		cases:  map[string]Case{"C1": {ID: "C1", State: "IN_PROGRESS", CreatedAt: now, UpdatedAt: now}},
		owners: map[string]Owner{"C1": {OUHandle: "cda", Roles: []string{"officer"}}},
	}
	// Every flow on a case files its tasks under the case ID.
	tasks := fakeTasks{"C1": {
		{TaskID: "n1", TaskType: "APPLICATION", State: "PENDING_USER", TaskTemplateID: "tpl-a"},
		{TaskID: "n2", TaskType: "APPLICATION", State: "COMPLETED", TaskTemplateID: "tpl-b"},
		{TaskID: "n3", TaskType: "SYSTEM", State: "COMPLETED"},
	}}
	return NewCaseHandler(repo, tasks, newSet(t, nil, nil), map[string]string{"officer": "Officer", "reviewer": "Reviewer"})
}

func TestCaseHandlerRequiresACatalogRole(t *testing.T) {
	h := newCaseHandler(t)
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
	h := newCaseHandler(t)
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

func TestCaseHandlerDetailListsTheCasesTasks(t *testing.T) {
	h := newCaseHandler(t)
	rec := httptest.NewRecorder()
	h.HandleGetCase(rec, requestBy(officer("Officer"), "/api/v1/cases/C1", "C1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got CaseDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.WorkflowNodes) != 2 {
		t.Fatalf("nodes = %+v, want n1 and n2 (SYSTEM skipped)", got.WorkflowNodes)
	}
	// Neither task's render config resolves, so each falls back to its template id.
	n1, n2 := got.WorkflowNodes[0], got.WorkflowNodes[1]
	if n1.ID != "n1" || n1.State != "IN_PROGRESS" || n1.WorkflowNodeTemplate.Name != "tpl-a" {
		t.Errorf("n1 = %+v", n1)
	}
	if n2.ID != "n2" || n2.State != "COMPLETED" || n2.WorkflowNodeTemplate.Name != "tpl-b" {
		t.Errorf("n2 = %+v", n2)
	}
}

func TestCaseHandlerUnknownCase(t *testing.T) {
	rec := httptest.NewRecorder()
	newCaseHandler(t).HandleGetCase(rec, requestBy(officer("Officer"), "/api/v1/cases/nope", "nope"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
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
