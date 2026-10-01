package agency

import (
	"context"
	"fmt"
)

// CompletionHandler is the parent workflow runner's completion callback;
// *consignment.Service satisfies it.
type CompletionHandler interface {
	CompletionHandler(workflowID string, finalContext map[string]any) error
}

// CompletionRouter sends completions of injected workflows to the agency and every
// other completion to next. Both kinds of workflow run on the same parent runner, and
// next (the consignment service) fails on a workflow id it has no consignment for.
type CompletionRouter struct {
	repo Repository
	next CompletionHandler
}

// NewCompletionRouter creates a CompletionRouter.
func NewCompletionRouter(repo Repository, next CompletionHandler) *CompletionRouter {
	return &CompletionRouter{repo: repo, next: next}
}

func (c *CompletionRouter) CompletionHandler(workflowID string, finalContext map[string]any) error {
	// The runner's callback carries no context; it runs inside a Temporal activity
	// that retries on error.
	found, err := c.repo.MarkCompleted(context.Background(), workflowID)
	if err != nil {
		return fmt.Errorf("agency: failed to record completion of %q: %w", workflowID, err)
	}
	if found {
		return nil
	}
	return c.next.CompletionHandler(workflowID, finalContext)
}
