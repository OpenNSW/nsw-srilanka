package cdn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Task templates the CDN lifecycle parks on while it waits for ASYCUDA. Each is
// resumed by the callback that answers it: §7.2 releases the integration wait,
// §7.3 releases the acknowledgment wait.
const (
	integrationWaitTemplateID    = "customs-cdn--integration-wait"
	acknowledgmentWaitTemplateID = "customs-wait-cdn-ack"
)

// stateQueuedExternally is the state a task parks in while an external system
// owns the next move. Only a task in that state is waiting to be resumed.
const stateQueuedExternally = "QUEUED_EXTERNALLY"

// resumeIntegrationWait releases the dispatch note's integration wait once §7.2
// arrives, so the CDN task completes and the acknowledgment wait that follows it
// opens. The edgeId is the correlator for the round-trip (§2.1), so the workflow
// is found by the edgeId its dispatch step recorded.
func (s *cdnWebhookService) resumeIntegrationWait(ctx context.Context, req CDNIntegrationResultRequest, alreadyProcessed bool) error {
	payload := map[string]any{
		"__command":  "submit",
		"integrated": req.Payload.Integrated,
	}
	if req.Payload.Integrated {
		ref := req.Payload.CDNRef
		payload["cdn_number"] = fmt.Sprintf("%s/%s/%s/%d", ref.Office, ref.Year, ref.Serial, ref.Number)
	} else {
		payload["error"] = describeErrors(req.Payload.Errors)
	}

	return s.resumeWait(ctx, waitLookup{
		edgeID:     req.Payload.EdgeID,
		templateID: integrationWaitTemplateID,
		payload:    payload,
	}, alreadyProcessed)
}

// resumeAcknowledgmentWait releases the acknowledgment wait once §7.3 arrives.
// That notification is correlated by cdnRef rather than edgeId, so the dispatch
// note is the bridge back to the workflow the wait belongs to.
func (s *cdnWebhookService) resumeAcknowledgmentWait(ctx context.Context, note *DispatchNote, alreadyAcknowledged bool) error {
	return s.resumeWait(ctx, waitLookup{
		edgeID:     note.EdgeID,
		templateID: acknowledgmentWaitTemplateID,
		payload: map[string]any{
			"__command":  "submit",
			"ack_status": string(DispatchNoteStatusAcknowledged),
		},
	}, alreadyAcknowledged)
}

type waitLookup struct {
	edgeID     string
	templateID string
	payload    map[string]any
}

// resumeWait finds the parked task the given callback answers and completes its
// step. A missing task is only an error when this callback is genuinely new: a
// redelivery (§2 retries up to four times) legitimately finds nothing left to
// resume, and must still be acknowledged so the retries stop.
func (s *cdnWebhookService) resumeWait(ctx context.Context, look waitLookup, alreadyProcessed bool) error {
	if s.db == nil || s.taskManager == nil {
		// A deployment without the workflow wiring still records the callback;
		// it simply has no task to advance.
		slog.DebugContext(ctx, "cdn: no workflow wiring, skipping task resume", "edge_id", look.edgeID)
		return nil
	}

	var record struct {
		ParentWorkflowID string `gorm:"column:parent_workflow_id"`
	}
	err := s.db.WithContext(ctx).
		Table("task_records_v2").
		// The dispatch step records the id under whichever key its output mapping
		// used; both spellings are in play across deployments.
		Where("data->'cig_cdn'->>'edge_id' = ? OR data->'cig_cdn'->>'edgeId' = ?", look.edgeID, look.edgeID).
		Select("parent_workflow_id").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if alreadyProcessed {
				slog.InfoContext(ctx, "cdn: no workflow for edgeId but callback already processed, ignoring redelivery",
					"edge_id", look.edgeID)
				return nil
			}
			return fmt.Errorf("edgeId %s: %w", look.edgeID, ErrDispatchNoteNotFoundByEdgeID)
		}
		return fmt.Errorf("failed to locate CDN workflow by edgeId %s: %w", look.edgeID, err)
	}

	var task struct {
		TaskID string `gorm:"column:task_id"`
	}
	err = s.db.WithContext(ctx).
		Table("task_records_v2").
		Where("parent_workflow_id = ? AND active_task_template_id = ? AND state = ?",
			record.ParentWorkflowID, look.templateID, stateQueuedExternally).
		Select("task_id").
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.InfoContext(ctx, "cdn: no parked task to resume",
				"edge_id", look.edgeID, "template", look.templateID, "workflow_id", record.ParentWorkflowID)
			return nil
		}
		return fmt.Errorf("failed to locate parked CDN task for workflow %s: %w", record.ParentWorkflowID, err)
	}

	if err := s.taskManager.CompleteTaskStep(ctx, task.TaskID, look.payload); err != nil {
		return fmt.Errorf("failed to complete task step for task %s: %w", task.TaskID, err)
	}

	slog.InfoContext(ctx, "cdn: resumed parked task",
		"task_id", task.TaskID, "template", look.templateID, "edge_id", look.edgeID)
	return nil
}
