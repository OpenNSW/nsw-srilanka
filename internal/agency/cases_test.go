package agency

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

type fakeCaseRepo struct {
	cases     map[string]Case
	workflows map[string][]CaseWorkflow // case id -> workflows
}

func (f fakeCaseRepo) ListCases(context.Context, int, int) ([]Case, int64, error) {
	out := make([]Case, 0, len(f.cases))
	for _, c := range f.cases {
		out = append(out, c)
	}
	return out, int64(len(out)), nil
}

func (f fakeCaseRepo) GetCase(_ context.Context, id string) (*Case, error) {
	c, ok := f.cases[id]
	if !ok {
		return nil, ErrCaseNotFound
	}
	return &c, nil
}

func (f fakeCaseRepo) ListWorkflows(_ context.Context, caseID string) ([]CaseWorkflow, error) {
	return f.workflows[caseID], nil
}

type fakeTasks map[string][]store.TaskRecord // case id -> tasks

func (f fakeTasks) GetAllTasks(_ context.Context, caseID string) []store.TaskRecord {
	return f[caseID]
}

func requestAs(roles []string, target string, id string) *http.Request {
	if roles == nil {
		return requestBy(nil, target, id)
	}
	return requestBy(&authn.Principal{Kind: authn.KindUser, Roles: roles}, target, id)
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

func TestCaseHandler(t *testing.T) {
	now := time.Now()
	repo := fakeCaseRepo{
		cases:     map[string]Case{"C1": {ID: "C1", State: "IN_PROGRESS", CreatedAt: now, UpdatedAt: now}},
		workflows: map[string][]CaseWorkflow{"C1": {{"task-1", "code-a", 1}, {"task-2", "code-unregistered", 1}}},
	}
	tasks := fakeTasks{
		"task-1": {{TaskID: "n1", TaskType: "APPLICATION", State: "PENDING_USER", TaskTemplateID: "tpl-a"}},
		"task-2": {
			{TaskID: "n2", TaskType: "APPLICATION", State: "COMPLETED", TaskTemplateID: "tpl"},
			{TaskID: "n3", TaskType: "SYSTEM", State: "COMPLETED"},
		},
	}
	reg := newSet(t, map[string]string{
		"a.json": `{"schemaVersion":2,"taskCode":"code-a","workflow":"wf-a","meta":{"title":"Verify","description":"Officer verifies"}}`,
	}, []artifact.ManifestRow{row("code-a", "task_config", "a.json")})
	h := NewCaseHandler(repo, tasks, reg, map[string]string{"officer": "Officer", "reviewer": "Reviewer"})

	t.Run("requires a catalog role", func(t *testing.T) {
		for name, tc := range map[string]struct {
			p    *authn.Principal
			want int
		}{
			"unauthenticated": {nil, http.StatusUnauthorized},
			"trader":          {&authn.Principal{Kind: authn.KindUser, Roles: []string{"Trader"}}, http.StatusForbidden},
			// Only a user can be an officer, whatever roles a client token carries.
			"client":        {&authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA", Roles: []string{"Officer"}}, http.StatusForbidden},
			"officer":       {&authn.Principal{Kind: authn.KindUser, Roles: []string{"Officer"}}, http.StatusOK},
			"reviewer only": {&authn.Principal{Kind: authn.KindUser, Roles: []string{"Reviewer"}}, http.StatusOK},
			"mixed roles":   {&authn.Principal{Kind: authn.KindUser, Roles: []string{"Trader", "Reviewer"}}, http.StatusOK},
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
	})

	t.Run("detail gathers tasks across the case's workflows", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.HandleGetCase(rec, requestAs([]string{"Officer"}, "/api/v1/cases/C1", "C1"))
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
		n1, n2 := got.WorkflowNodes[0], got.WorkflowNodes[1]
		// task-1's code has a task_config, so its meta wins over the render title;
		// task-2's does not, so it falls back to the template id.
		tpl1 := n1.WorkflowNodeTemplate
		if n1.ID != "n1" || n1.State != "IN_PROGRESS" || tpl1.Name != "Verify" || tpl1.Description != "Officer verifies" {
			t.Errorf("n1 = %+v", n1)
		}
		if n2.ID != "n2" || n2.State != "COMPLETED" || n2.WorkflowNodeTemplate.Name != "tpl" {
			t.Errorf("n2 = %+v", n2)
		}
	})

	t.Run("unknown case is 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.HandleGetCase(rec, requestAs([]string{"Officer"}, "/api/v1/cases/nope", "nope"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

// A catalog entry mapped to "" must not match a caller whose roles hold an empty string.
func TestCaseHandlerEmptyTokenRole(t *testing.T) {
	h := NewCaseHandler(fakeCaseRepo{}, fakeTasks{}, newSet(t, nil, nil), map[string]string{"reviewer": ""})
	for _, roles := range [][]string{{""}, {"Reviewer"}, nil} {
		rec := httptest.NewRecorder()
		h.HandleListCases(rec, requestBy(&authn.Principal{Kind: authn.KindUser, Roles: roles}, "/api/v1/cases", ""))
		if rec.Code != http.StatusForbidden {
			t.Errorf("roles %q: status = %d, want 403", roles, rec.Code)
		}
	}
}
