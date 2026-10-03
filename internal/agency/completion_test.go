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

func TestCompletionHandler(t *testing.T) {
	t.Run("injected workflow is marked completed", func(t *testing.T) {
		repo := &completionRepo{injected: map[string]bool{"task-1": true}}
		if err := NewCompletionHandler(repo).CompletionHandler("task-1", nil); err != nil {
			t.Fatal(err)
		}
		if len(repo.completed) != 1 || repo.completed[0] != "task-1" {
			t.Fatalf("completed = %v", repo.completed)
		}
	})

	t.Run("unknown workflow is an error", func(t *testing.T) {
		repo := &completionRepo{}
		if err := NewCompletionHandler(repo).CompletionHandler("consignment-1", nil); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("a lookup failure is returned", func(t *testing.T) {
		repo := &completionRepo{err: errors.New("db down")}
		if err := NewCompletionHandler(repo).CompletionHandler("task-1", nil); err == nil {
			t.Fatal("want error")
		}
	})
}
