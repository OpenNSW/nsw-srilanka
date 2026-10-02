package agency

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
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
	reg := artifact.NewRegistry(testutil.MemLoader{})
	svc := NewService(recordingRepo{t: t}, reg, nil)

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
	reg := artifact.NewRegistry(testutil.MemLoader{
		"verify.json": []byte(`{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf","meta":{"title":"Verify"}}`),
		"letter.json": []byte(`{"schemaVersion":2,"taskCode":"letter","workflow":"letter_wf","meta":{"title":"Letter"}}`),
	})
	reg.RegisterArtifact("verify", "task_config", "", "verify.json")
	reg.RegisterArtifact("letter", "task_config", "", "letter.json")
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
