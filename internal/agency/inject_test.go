package agency

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	workflow "github.com/OpenNSW/core/workflow"
)

// recordingRepo fails the test if Inject gets as far as recording anything.
type recordingRepo struct {
	Repository
	t *testing.T
}

func (r recordingRepo) Record(context.Context, Workflow) error {
	r.t.Fatal("Record called for an unknown task code")
	return nil
}

func (recordingRepo) Get(context.Context, string) (*Workflow, error) { return nil, ErrWorkflowNotFound }

func TestInjectUnknownTaskCode(t *testing.T) {
	set := newSet(t, nil, nil)
	svc := NewService(recordingRepo{t: t}, set, nil)

	_, err := svc.Inject(context.Background(), InjectRequest{TaskID: "t1", TaskCode: "nope", ConsignmentID: "C1"})
	if !errors.Is(err, ErrUnknownTaskCode) {
		t.Fatalf("err = %v, want ErrUnknownTaskCode", err)
	}
}

// storedRepo holds one already-recorded row: Record is the ON CONFLICT no-op and Get
// returns the row.
type storedRepo struct {
	Repository
	row Workflow
}

func (r storedRepo) Record(context.Context, Workflow) error { return nil }

func (r storedRepo) Get(context.Context, string) (*Workflow, error) {
	w := r.row
	return &w, nil
}

func TestInjectRepeatTaskID(t *testing.T) {
	reg := newSet(t, map[string]string{
		"verify.json": `{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf","meta":{"title":"Verify"}}`,
		"letter.json": `{"schemaVersion":2,"taskCode":"letter","workflow":"letter_wf","meta":{"title":"Letter"}}`,
	}, []artifact.ManifestRow{row("verify", "task_config", "verify.json"), row("letter", "task_config", "letter.json")})
	stored := Workflow{TaskID: "T1", TaskCode: "verify", CaseID: "C1", ManifestVersion: 1, Status: StatusStarted}

	// The workflow manager is nil: reaching the engine would panic, so these also
	// prove no repeat starts a workflow.
	for name, tc := range map[string]struct {
		status Status
		req    InjectRequest
		want   error
	}{
		"different consignmentId": {StatusStarted, InjectRequest{TaskID: "T1", TaskCode: "verify", ConsignmentID: "C2"}, ErrConflict},
		"different taskCode":      {StatusStarted, InjectRequest{TaskID: "T1", TaskCode: "letter", ConsignmentID: "C1"}, ErrConflict},
		// The retry that could have started letter_wf under a verify row.
		"different taskCode while starting": {StatusStarting, InjectRequest{TaskID: "T1", TaskCode: "letter", ConsignmentID: "C1"}, ErrConflict},
		"same details":                      {StatusStarted, InjectRequest{TaskID: "T1", TaskCode: "verify", ConsignmentID: "C1"}, nil},
	} {
		r := stored
		r.Status = tc.status
		got, err := NewService(storedRepo{row: r}, reg, nil).Inject(context.Background(), tc.req)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
			continue
		}
		if tc.want == nil && (got == nil || got.CaseID != "C1") {
			t.Errorf("%s: got %+v, want the stored row", name, got)
		}
	}
}

// startingRepo records the first Workflow it is given and returns it from Get as the
// recorded row, still STARTING, so Inject goes on to start it.
type startingRepo struct {
	Repository
	recorded *Workflow
}

func (r *startingRepo) Record(_ context.Context, w Workflow) error {
	if r.recorded == nil {
		w.Status = StatusStarting
		r.recorded = &w
	}
	return nil
}

func (r *startingRepo) Get(context.Context, string) (*Workflow, error) {
	if r.recorded == nil {
		return nil, ErrWorkflowNotFound
	}
	w := *r.recorded
	return &w, nil
}

func (r *startingRepo) MarkStarted(context.Context, string) error { return nil }

// startCapture records the definition and variables each workflow was started with.
type startCapture struct {
	workflow.Manager
	defs []workflow.WorkflowDefinition
	vars []map[string]any
}

func (m *startCapture) StartWorkflow(_ context.Context, _ string, def workflow.WorkflowDefinition, vars map[string]any) error {
	m.defs = append(m.defs, def)
	m.vars = append(m.vars, vars)
	return nil
}

func TestInjectSeedsCallbackToken(t *testing.T) {
	reg := newSet(t, map[string]string{
		"verify.json":    `{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf","meta":{"title":"Verify"}}`,
		"verify_wf.json": `{"id":"verify_wf","nodes":[],"edges":[]}`,
	}, []artifact.ManifestRow{row("verify", "task_config", "verify.json"), row("verify_wf", "workflow", "verify_wf.json")})
	req := InjectRequest{TaskID: "T1", TaskCode: "verify", ConsignmentID: "C1"}

	t.Run("token is seeded from the recorded row", func(t *testing.T) {
		repo, wm := &startingRepo{}, &startCapture{}
		svc := NewService(repo, reg, wm)

		first, retry := req, req
		first.CallbackToken = "tok-1"
		// A retried start runs with the first inject's token, like its payload.
		retry.CallbackToken = "tok-2"
		for _, r := range []InjectRequest{first, retry} {
			if _, err := svc.Inject(context.Background(), r); err != nil {
				t.Fatal(err)
			}
		}
		if repo.recorded.CallbackToken != "tok-1" {
			t.Errorf("recorded token = %q, want tok-1", repo.recorded.CallbackToken)
		}
		// The row is never marked started, so the retry starts it again.
		if len(wm.vars) != 2 {
			t.Fatalf("started %d times, want 2", len(wm.vars))
		}
		for i, vars := range wm.vars {
			if vars["callbackToken"] != "tok-1" {
				t.Errorf("start %d: callbackToken = %v, want tok-1", i, vars["callbackToken"])
			}
		}
	})

	t.Run("no token leaves the variable unset", func(t *testing.T) {
		wm := &startCapture{}
		if _, err := NewService(&startingRepo{}, reg, wm).Inject(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if _, ok := wm.vars[0]["callbackToken"]; ok {
			t.Errorf("callbackToken = %v, want unset", wm.vars[0]["callbackToken"])
		}
	})
}

// versionedSet has two manifests; v2, the current one, points verify at a config whose
// workflow is verify_wf_v2.
func versionedSet(t *testing.T) *artifact.Set {
	files := map[string]string{
		"verify.json":       `{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf","meta":{"title":"Verify"}}`,
		"verify_v2.json":    `{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf_v2","meta":{"title":"Verify"}}`,
		"verify_wf.json":    `{"id":"verify_wf","nodes":[],"edges":[]}`,
		"verify_wf_v2.json": `{"id":"verify_wf_v2","nodes":[],"edges":[]}`,
	}
	v1 := []artifact.ManifestRow{row("verify", "task_config", "verify.json"), row("verify_wf", "workflow", "verify_wf.json")}
	v2 := []artifact.ManifestRow{row("verify", "task_config", "verify_v2.json"), row("verify_wf_v2", "workflow", "verify_wf_v2.json")}
	return newSet(t, files, v1, v2)
}

// A new inject runs on the current manifest: it is recorded with it and started with it.
func TestInjectPinsTheCurrentManifestVersion(t *testing.T) {
	repo, wm := &startingRepo{}, &startCapture{}
	if _, err := NewService(repo, versionedSet(t), wm).Inject(context.Background(), InjectRequest{TaskID: "T1", TaskCode: "verify", ConsignmentID: "C1"}); err != nil {
		t.Fatal(err)
	}
	if repo.recorded.ManifestVersion != 2 {
		t.Errorf("recorded manifest version = %d, want 2", repo.recorded.ManifestVersion)
	}
	if len(wm.defs) != 1 || wm.defs[0].ID != "verify_wf_v2" {
		t.Fatalf("started %+v, want verify_wf_v2", wm.defs)
	}
	if got := wm.vars[0][workflow.VarManifestVersion]; got != 2 {
		t.Errorf("%s = %v, want 2", workflow.VarManifestVersion, got)
	}
}

// A retry of an inject recorded before a deploy starts the workflow its manifest named.
func TestInjectRetryKeepsTheRecordedManifestVersion(t *testing.T) {
	repo := &startingRepo{recorded: &Workflow{TaskID: "T1", TaskCode: "verify", CaseID: "C1", ManifestVersion: 1, Status: StatusStarting}}
	wm := &startCapture{}
	if _, err := NewService(repo, versionedSet(t), wm).Inject(context.Background(), InjectRequest{TaskID: "T1", TaskCode: "verify", ConsignmentID: "C1"}); err != nil {
		t.Fatal(err)
	}
	if len(wm.defs) != 1 || wm.defs[0].ID != "verify_wf" {
		t.Fatalf("started %+v, want the v1 workflow verify_wf", wm.defs)
	}
	if got := wm.vars[0][workflow.VarManifestVersion]; got != 1 {
		t.Errorf("%s = %v, want 1", workflow.VarManifestVersion, got)
	}
}

// A retry of an inject recorded on a manifest whose task config the current one dropped
// still starts it, on the recorded version.
func TestInjectRetryOfATaskCodeTheCurrentManifestDropped(t *testing.T) {
	files := map[string]string{
		"old.json":    `{"schemaVersion":2,"taskCode":"old","workflow":"old_wf","meta":{"title":"Old"}}`,
		"old_wf.json": `{"id":"old_wf","nodes":[],"edges":[]}`,
		"new.json":    `{"schemaVersion":2,"taskCode":"new","workflow":"new_wf","meta":{"title":"New"}}`,
		"new_wf.json": `{"id":"new_wf","nodes":[],"edges":[]}`,
	}
	set := newSet(t, files,
		[]artifact.ManifestRow{row("old", "task_config", "old.json"), row("old_wf", "workflow", "old_wf.json")},
		[]artifact.ManifestRow{row("new", "task_config", "new.json"), row("new_wf", "workflow", "new_wf.json")})

	repo := &startingRepo{recorded: &Workflow{TaskID: "T1", TaskCode: "old", CaseID: "C1", ManifestVersion: 1, Status: StatusStarting}}
	wm := &startCapture{}
	if _, err := NewService(repo, set, wm).Inject(context.Background(), InjectRequest{TaskID: "T1", TaskCode: "old", ConsignmentID: "C1"}); err != nil {
		t.Fatal(err)
	}
	if len(wm.defs) != 1 || wm.defs[0].ID != "old_wf" {
		t.Fatalf("started %+v, want old_wf", wm.defs)
	}

	// A new inject of that task code is refused: the current manifest does not hold it.
	if _, err := NewService(&startingRepo{}, set, &startCapture{}).Inject(context.Background(), InjectRequest{TaskID: "T2", TaskCode: "old", ConsignmentID: "C1"}); !errors.Is(err, ErrUnknownTaskCode) {
		t.Fatalf("new inject of a dropped task code: err = %v, want ErrUnknownTaskCode", err)
	}
}
