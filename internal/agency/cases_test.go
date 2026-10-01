package agency

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/taskflow/store"
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

type fakeTasks map[string][]store.TaskRecord // root workflow id -> tasks

func (f fakeTasks) GetAllTasks(_ context.Context, rootWorkflowID string) []store.TaskRecord {
	return f[rootWorkflowID]
}

func requestAs(roles []string, target string, id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if id != "" {
		r.SetPathValue("id", id)
	}
	if roles != nil {
		ac := &authn.AuthContext{User: &authn.UserContext{Roles: roles}}
		r = r.WithContext(context.WithValue(r.Context(), authn.AuthContextKey, ac))
	}
	return r
}

func TestCaseHandler(t *testing.T) {
	now := time.Now()
	repo := fakeCaseRepo{
		cases:     map[string]Case{"C1": {ID: "C1", State: "IN_PROGRESS", CreatedAt: now, UpdatedAt: now}},
		workflows: map[string][]CaseWorkflow{"C1": {{"task-1", "code-a"}, {"task-2", "code-unregistered"}}},
	}
	tasks := fakeTasks{
		"task-1": {{TaskID: "n1", TaskType: "APPLICATION", State: "PENDING_USER", RenderConfig: json.RawMessage(`{"title":"Review"}`)}},
		"task-2": {
			{TaskID: "n2", TaskType: "APPLICATION", State: "COMPLETED", ActiveTaskTemplateID: "tpl"},
			{TaskID: "n3", TaskType: "SYSTEM", State: "COMPLETED"},
		},
	}
	reg := artifact.NewRegistry(testutil.MemLoader{
		"a.json": []byte(`{"schemaVersion":2,"taskCode":"code-a","workflow":"wf-a","meta":{"title":"Verify","description":"Officer verifies"}}`),
	})
	reg.RegisterArtifact("code-a", "task_config", "", "a.json")
	h := NewCaseHandler(repo, tasks, reg, "Officer")

	t.Run("requires the officer role", func(t *testing.T) {
		for name, tc := range map[string]struct {
			roles []string
			want  int
		}{
			"unauthenticated": {nil, http.StatusUnauthorized},
			"trader":          {[]string{"Trader"}, http.StatusForbidden},
		} {
			rec := httptest.NewRecorder()
			h.HandleListCases(rec, requestAs(tc.roles, "/api/v1/cases", ""))
			if rec.Code != tc.want {
				t.Errorf("%s: status = %d, want %d", name, rec.Code, tc.want)
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
