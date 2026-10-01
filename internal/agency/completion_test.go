package agency

import (
	"context"
	"errors"
	"testing"
)

type completionRepo struct {
	Repository
	injected  map[string]bool
	err       error
	completed []string
}

func (r *completionRepo) MarkCompleted(_ context.Context, taskID string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	if !r.injected[taskID] {
		return false, nil
	}
	r.completed = append(r.completed, taskID)
	return true, nil
}

type nextHandler struct{ called []string }

func (n *nextHandler) CompletionHandler(workflowID string, _ map[string]any) error {
	n.called = append(n.called, workflowID)
	return nil
}

func TestCompletionRouter(t *testing.T) {
	t.Run("injected workflow completes in the agency, not the next handler", func(t *testing.T) {
		repo := &completionRepo{injected: map[string]bool{"task-1": true}}
		next := &nextHandler{}
		if err := NewCompletionRouter(repo, next).CompletionHandler("task-1", nil); err != nil {
			t.Fatal(err)
		}
		if len(repo.completed) != 1 || len(next.called) != 0 {
			t.Fatalf("completed = %v, next called = %v", repo.completed, next.called)
		}
	})

	t.Run("other workflows go to the next handler", func(t *testing.T) {
		repo := &completionRepo{}
		next := &nextHandler{}
		if err := NewCompletionRouter(repo, next).CompletionHandler("consignment-1", nil); err != nil {
			t.Fatal(err)
		}
		if len(next.called) != 1 || next.called[0] != "consignment-1" {
			t.Fatalf("next called = %v", next.called)
		}
	})

	t.Run("a lookup failure is returned, not passed on", func(t *testing.T) {
		repo := &completionRepo{err: errors.New("db down")}
		next := &nextHandler{}
		if err := NewCompletionRouter(repo, next).CompletionHandler("task-1", nil); err == nil {
			t.Fatal("want error")
		}
		if len(next.called) != 0 {
			t.Fatalf("next called = %v, want none", next.called)
		}
	})
}
