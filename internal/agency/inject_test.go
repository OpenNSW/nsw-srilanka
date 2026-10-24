package agency

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
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

func TestInjectUnknownTaskCode(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{"manifest_v1.json": []byte(`{"artifacts":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(recordingRepo{t: t}, set, nil)

	_, err = svc.Inject(context.Background(), InjectRequest{TaskID: "t1", TaskCode: "nope", ConsignmentID: "C1"})
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
	reg := testSet(t, testutil.MemLoader{
		"verify.json":      []byte(`{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf","meta":{"title":"Verify"}}`),
		"letter.json":      []byte(`{"schemaVersion":2,"taskCode":"letter","workflow":"letter_wf","meta":{"title":"Letter"}}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"verify","kind":"task_config","path":"verify.json"},{"id":"letter","kind":"task_config","path":"letter.json"}]}`),
	})
	row := Workflow{TaskID: "T1", TaskCode: "verify", CaseID: "C1", Status: StatusStarted}

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
		r := row
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
	w := *r.recorded
	return &w, nil
}

func (r *startingRepo) MarkStarted(context.Context, string) error { return nil }

// startCapture records the variables each workflow was started with.
type startCapture struct {
	workflow.Manager
	vars []map[string]any
}

func (m *startCapture) StartWorkflow(_ context.Context, _ string, _ workflow.WorkflowDefinition, vars map[string]any) error {
	m.vars = append(m.vars, vars)
	return nil
}

func TestInjectSeedsCallbackToken(t *testing.T) {
	reg := testSet(t, testutil.MemLoader{
		"verify.json":      []byte(`{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf","meta":{"title":"Verify"}}`),
		"verify_wf.json":   []byte(`{"id":"verify_wf","nodes":[],"edges":[]}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"verify","kind":"task_config","path":"verify.json"},{"id":"verify_wf","kind":"workflow","path":"verify_wf.json"}]}`),
	})
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
			if vars[workflow.VarManifestVersion] != 1 {
				t.Errorf("start %d: %s = %v, want the current manifest version 1", i, workflow.VarManifestVersion, vars[workflow.VarManifestVersion])
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

// testSet loads files, which must include manifest_v1.json, as a one-manifest artifact set.
func testSet(t *testing.T, files testutil.MemLoader) *artifact.Set {
	t.Helper()
	set, err := artifact.LoadSet(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	return set
}
