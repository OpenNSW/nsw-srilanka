package systemcaller

import (
	"context"
	"reflect"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// recorder records what reached the task manager.
type recorder struct {
	ctx            context.Context
	taskID, stepID string
	token          string
	payload        map[string]any
}

func (r *recorder) CompleteTaskStep(ctx context.Context, taskID, stepID string, payload map[string]any) error {
	r.ctx, r.taskID, r.stepID, r.payload = ctx, taskID, stepID, payload
	return nil
}

func (r *recorder) CompleteTaskStepByToken(ctx context.Context, token string, payload map[string]any) error {
	r.ctx, r.token, r.payload = ctx, token, payload
	return nil
}

func systemOnly(t *testing.T, ctx context.Context) {
	t.Helper()
	in, ok := taskauthz.InputFromContext(ctx)
	if !ok || in.Kind != taskauthz.KindSystem || in.Roles != nil || in.ClientID != "" || in.OwnedRoles != nil {
		t.Errorf("the task manager saw %+v, %v; want only the system principal", in, ok)
	}
}

// Both ways in reach the task manager as the system principal and nothing else,
// even when the caller's context already carried a user.
func TestCompleter(t *testing.T) {
	user := taskauthz.WithInput(context.Background(), taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}})
	payload := map[string]any{"__command": "submit", "release_status": "RELEASED"}

	t.Run("by ids", func(t *testing.T) {
		rec := &recorder{}
		if err := New(rec).CompleteTaskStep(user, "t1", "s1", payload); err != nil {
			t.Fatal(err)
		}
		systemOnly(t, rec.ctx)
		if rec.taskID != "t1" || rec.stepID != "s1" || !reflect.DeepEqual(rec.payload, payload) {
			t.Errorf("passed on (%q, %q, %v), want the call unchanged", rec.taskID, rec.stepID, rec.payload)
		}
	})

	t.Run("by token", func(t *testing.T) {
		rec := &recorder{}
		if err := New(rec).CompleteTaskStepByToken(user, "tok", payload); err != nil {
			t.Fatal(err)
		}
		systemOnly(t, rec.ctx)
		if rec.token != "tok" || !reflect.DeepEqual(rec.payload, payload) {
			t.Errorf("passed on (%q, %v), want the call unchanged", rec.token, rec.payload)
		}
	})
}
