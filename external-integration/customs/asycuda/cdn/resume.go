package cdn

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/parkedtask"
)

// Task templates the CDN lifecycle parks on while it waits for ASYCUDA. Each is
// resumed by the callback that answers it: §7.2 releases the integration wait,
// §7.3 releases the acknowledgment wait.
const (
	integrationWaitTemplateID    = "customs-cdn--integration-wait"
	acknowledgmentWaitTemplateID = "customs-wait-cdn-ack"
)

// resumeIntegrationWait releases the dispatch note's integration wait once §7.2
// arrives, so the CDN task completes and the acknowledgment wait that follows it
// opens. The edgeId is the correlator for the round-trip (§2.1): the dispatch
// note it names recorded the workflow that sent it.
func (s *cdnWebhookService) resumeIntegrationWait(ctx context.Context, note *DispatchNote, req CDNIntegrationResultRequest, alreadyProcessed bool) error {
	payload := map[string]any{
		"__command":  "submit",
		"integrated": req.Payload.Integrated,
	}
	if req.Payload.Integrated {
		ref := req.Payload.CDNRef
		payload["cdn_number"] = formatCDNRef(ref)
	} else {
		payload["error"] = describeErrors(req.Payload.Errors)
	}

	return s.resumeWait(ctx, waitLookup{
		note:       note,
		templateID: integrationWaitTemplateID,
		payload:    payload,
	}, alreadyProcessed)
}

// resumeAcknowledgmentWait releases the acknowledgment wait once §7.3 arrives.
// That notification is correlated by cdnRef rather than edgeId, so the dispatch
// note is the bridge back to the workflow the wait belongs to.
func (s *cdnWebhookService) resumeAcknowledgmentWait(ctx context.Context, note *DispatchNote, alreadyAcknowledged bool) error {
	return s.resumeWait(ctx, waitLookup{
		note:       note,
		templateID: acknowledgmentWaitTemplateID,
		payload: map[string]any{
			"__command":  "submit",
			"ack_status": string(DispatchNoteStatusAcknowledged),
		},
	}, alreadyAcknowledged)
}

type waitLookup struct {
	note       *DispatchNote
	templateID string
	payload    map[string]any
}

// resumeWait finds the parked task the given callback answers and completes its
// step. A missing task is only an error when this callback is genuinely new —
// its task may not have parked yet, so SLC Edge is asked to retry. A redelivery
// (§2 retries up to four times) legitimately finds nothing left to resume, and
// must still be acknowledged so the retries stop. Tasks that could not be read
// are always an error: whether one is parked is not known.
func (s *cdnWebhookService) resumeWait(ctx context.Context, look waitLookup, alreadyProcessed bool) error {
	edgeID := look.note.EdgeID
	if s.tasks == nil || s.taskManager == nil {
		// A deployment without the workflow wiring still records the callback;
		// it simply has no task to advance.
		slog.DebugContext(ctx, "cdn: no workflow wiring, skipping task resume", "edge_id", edgeID)
		return nil
	}

	if look.note.WorkflowID == "" {
		if alreadyProcessed {
			slog.InfoContext(ctx, "cdn: no workflow for edgeId but callback already processed, ignoring redelivery",
				"edge_id", edgeID)
			return nil
		}
		return fmt.Errorf("edgeId %s: %w", edgeID, ErrDispatchNoteNotFoundByEdgeID)
	}

	task, ok, err := parkedtask.Find(ctx, s.tasks, look.note.CaseID, look.note.WorkflowID, look.templateID)
	if err != nil {
		return fmt.Errorf("locate the %s task for workflow %s: %w", look.templateID, look.note.WorkflowID, err)
	}
	if !ok {
		if alreadyProcessed {
			slog.InfoContext(ctx, "cdn: no parked task to resume, callback already processed",
				"edge_id", edgeID, "template", look.templateID, "workflow_id", look.note.WorkflowID)
			return nil
		}
		return fmt.Errorf("edgeId %s, template %s: %w", edgeID, look.templateID, ErrTaskNotParked)
	}

	if err := s.taskManager.CompleteTaskStep(ctx, task.TaskID, task.StepID, look.payload); err != nil {
		return fmt.Errorf("failed to complete task step for task %s: %w", task.TaskID, err)
	}

	slog.InfoContext(ctx, "cdn: resumed parked task",
		"task_id", task.TaskID, "template", look.templateID, "edge_id", edgeID)
	return nil
}
