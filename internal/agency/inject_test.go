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
