package caseflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	workflow "github.com/OpenNSW/core/workflow"
)

func newRegistry() *artifact.Registry {
	reg := artifact.NewRegistry(testutil.MemLoader{
		"export.json": []byte(`{"id":"export","nodes":[],"edges":[]}`),
		"notice.json": []byte(`{"id":"notice","nodes":[],"edges":[]}`),
	})
	reg.RegisterArtifact("export", "workflow", "", "export.json")
	reg.RegisterArtifact("notice", "workflow", "", "notice.json")
	return reg
}

// memRepo is an in-memory Repository with Record's first-sight semantics.
type memRepo struct {
	flows     map[string]*Flow
	completed []string
}

func newMemRepo(rows ...Flow) *memRepo {
	r := &memRepo{flows: map[string]*Flow{}}
	for i := range rows {
		f := rows[i]
		r.flows[f.WorkflowID] = &f
	}
	return r
}

func (r *memRepo) EnsureCase(context.Context, string) error { return nil }

func (r *memRepo) Record(_ context.Context, f Flow) error {
	if _, ok := r.flows[f.WorkflowID]; !ok {
		f.Status = StatusStarting
		r.flows[f.WorkflowID] = &f
	}
	return nil
}

func (r *memRepo) MarkStarted(_ context.Context, workflowID string) error {
	f, ok := r.flows[workflowID]
	if !ok {
		return ErrFlowNotFound
	}
	if f.Status == StatusStarting {
		f.Status = StatusStarted
	}
	return nil
}

func (r *memRepo) Get(_ context.Context, workflowID string) (*Flow, error) {
	f, ok := r.flows[workflowID]
	if !ok {
		return nil, ErrFlowNotFound
	}
	c := *f
	return &c, nil
}

func (r *memRepo) ListByCase(context.Context, string) ([]Flow, error) { return nil, nil }

func (r *memRepo) MarkCompleted(_ context.Context, workflowID string) (bool, error) {
	f, ok := r.flows[workflowID]
	if !ok {
		return false, nil
	}
	f.Status = StatusCompleted
	r.completed = append(r.completed, workflowID)
	return true, nil
}

type start struct {
	id   string
	vars map[string]any
}

// startCapture records each workflow start.
type startCapture struct {
	workflow.Manager
	starts []start
	err    error
}

func (m *startCapture) StartWorkflow(_ context.Context, id string, _ workflow.WorkflowDefinition, vars map[string]any) error {
	m.starts = append(m.starts, start{id: id, vars: vars})
	return m.err
}

func newService(t *testing.T, repo Repository, wm workflow.Manager) *Service {
	t.Helper()
	svc := NewService(repo, newRegistry())
	if wm != nil {
		if err := svc.RegisterWorkflowManager(wm); err != nil {
			t.Fatal(err)
		}
	}
	return svc
}

func TestStartFlowBuildsWorkflowID(t *testing.T) {
	repo, wm := newMemRepo(), &startCapture{}
	f, err := newService(t, repo, wm).StartFlow(context.Background(), StartRequest{CaseID: "C1", TemplateID: "export"})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(f.WorkflowID, ":")
	if len(parts) != 3 || parts[0] != "C1" || parts[1] != "export" || parts[2] == "" {
		t.Errorf("workflow ID = %q, want C1:export:<uuid>", f.WorkflowID)
	}
	if f.Status != StatusStarted || repo.flows[f.WorkflowID].Status != StatusStarted {
		t.Errorf("status = %s, recorded %s; want STARTED", f.Status, repo.flows[f.WorkflowID].Status)
	}
	if len(wm.starts) != 1 || wm.starts[0].id != f.WorkflowID {
		t.Fatalf("starts = %+v, want one start of %q", wm.starts, f.WorkflowID)
	}
	if got := wm.starts[0].vars[workflow.VarCaseID]; got != "C1" {
		t.Errorf("%s = %v, want the case ID", workflow.VarCaseID, got)
	}
}

func TestStartFlowSeedsRecordedVariables(t *testing.T) {
	repo, wm := newMemRepo(), &startCapture{err: errors.New("engine down")}
	svc := newService(t, repo, wm)
	req := StartRequest{CaseID: "C1", TemplateID: "notice", WorkflowID: "C1:notice:ev1", Vars: map[string]any{"ref": "first"}}

	if _, err := svc.StartFlow(context.Background(), req); err == nil {
		t.Fatal("want the engine's error")
	}
	if repo.flows["C1:notice:ev1"].Status != StatusStarting {
		t.Fatalf("status after a failed start = %s, want STARTING", repo.flows["C1:notice:ev1"].Status)
	}

	// The retry carries different variables; the flow still starts with the first ones.
	wm.err = nil
	req.Vars = map[string]any{"ref": "second"}
	if _, err := svc.StartFlow(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(wm.starts) != 2 {
		t.Fatalf("started %d times, want 2", len(wm.starts))
	}
	for i, s := range wm.starts {
		if s.vars["ref"] != "first" {
			t.Errorf("start %d: ref = %v, want first", i, s.vars["ref"])
		}
	}
}

func TestStartFlowRepeat(t *testing.T) {
	recorded := func(status Status) Flow {
		return Flow{WorkflowID: "C1:notice:ev1", CaseID: "C1", TemplateID: "notice", Status: status}
	}
	// The workflow manager is nil: reaching the engine fails, so these also prove no
	// repeat starts a workflow.
	for name, tc := range map[string]struct {
		row  Flow
		req  StartRequest
		want error
	}{
		"different case":     {recorded(StatusStarted), StartRequest{CaseID: "C2", TemplateID: "notice", WorkflowID: "C1:notice:ev1"}, ErrConflict},
		"different template": {recorded(StatusStarted), StartRequest{CaseID: "C1", TemplateID: "export", WorkflowID: "C1:notice:ev1"}, ErrConflict},
		// The retry that could have started export under a notice row.
		"different template while starting": {recorded(StatusStarting), StartRequest{CaseID: "C1", TemplateID: "export", WorkflowID: "C1:notice:ev1"}, ErrConflict},
		"started":                           {recorded(StatusStarted), StartRequest{CaseID: "C1", TemplateID: "notice", WorkflowID: "C1:notice:ev1"}, nil},
		"completed":                         {recorded(StatusCompleted), StartRequest{CaseID: "C1", TemplateID: "notice", WorkflowID: "C1:notice:ev1"}, nil},
	} {
		got, err := newService(t, newMemRepo(tc.row), nil).StartFlow(context.Background(), tc.req)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
			continue
		}
		if tc.want == nil && (got == nil || got.Status != tc.row.Status) {
			t.Errorf("%s: got %+v, want the recorded row", name, got)
		}
	}
}

func TestStartFlowRejects(t *testing.T) {
	for name, tc := range map[string]struct {
		req  StartRequest
		want error
	}{
		"unknown template": {StartRequest{CaseID: "C1", TemplateID: "nope"}, ErrUnknownTemplate},
		"no case":          {StartRequest{TemplateID: "export"}, ErrInvalidRequest},
		"slash in ID":      {StartRequest{CaseID: "C1", TemplateID: "export", WorkflowID: "C1/export/1"}, ErrInvalidRequest},
		"vars not JSON":    {StartRequest{CaseID: "C1", TemplateID: "export", Vars: map[string]any{"f": func() {}}}, ErrInvalidRequest},
	} {
		repo := newMemRepo()
		_, err := newService(t, repo, &startCapture{}).StartFlow(context.Background(), tc.req)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		if len(repo.flows) != 0 {
			t.Errorf("%s: recorded %d flows, want none", name, len(repo.flows))
		}
	}
}

func TestStartFlowRecordsPayload(t *testing.T) {
	repo := newMemRepo()
	f, err := newService(t, repo, &startCapture{}).StartFlow(context.Background(),
		StartRequest{CaseID: "C1", TemplateID: "export", Vars: map[string]any{"n": 1}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(repo.flows[f.WorkflowID].Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["n"] != float64(1) {
		t.Errorf("payload = %s, want {\"n\":1}", repo.flows[f.WorkflowID].Payload)
	}
}

func TestCompletionHandler(t *testing.T) {
	repo := newMemRepo(Flow{WorkflowID: "W1", CaseID: "C1", TemplateID: "export", Status: StatusStarted})
	svc := newService(t, repo, nil)

	if err := svc.CompletionHandler("W1", nil); err != nil {
		t.Fatal(err)
	}
	if repo.flows["W1"].Status != StatusCompleted {
		t.Errorf("status = %s, want COMPLETED", repo.flows["W1"].Status)
	}
	if err := svc.CompletionHandler("unknown", nil); err == nil {
		t.Error("completion of an unrecorded workflow: want an error")
	}
}
