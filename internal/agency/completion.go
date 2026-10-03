package agency

import (
	"context"
	"fmt"
)

// CompletionHandler is the parent workflow runner's completion callback in agency
// mode. Every parent workflow an agency runs was injected, so a completion marks its
// workflow COMPLETED and, once every workflow of its case is, the case FINISHED.
type CompletionHandler struct {
	repo Repository
}

// NewCompletionHandler creates a CompletionHandler.
func NewCompletionHandler(repo Repository) *CompletionHandler {
	return &CompletionHandler{repo: repo}
}

// CompletionHandler records the completion of workflowID. The runner's callback
// carries no context; it runs inside a Temporal activity that retries on error.
func (c *CompletionHandler) CompletionHandler(workflowID string, _ map[string]any) error {
	found, err := c.repo.MarkCompleted(context.Background(), workflowID)
	if err != nil {
		return fmt.Errorf("agency: failed to record completion of %q: %w", workflowID, err)
	}
	if !found {
		// Nothing else runs parent workflows in agency mode, so this is a bug or a
		// workflow from another deployment sharing the Temporal namespace.
		return fmt.Errorf("agency: completed workflow %q is not an injected workflow", workflowID)
	}
	return nil
}
