package cdn

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/parkedtask"
)

// CDNWebhookService defines the use-case layer for processing asynchronous
// ASYCUDA callbacks related to Cargo Dispatch Notes.
type CDNWebhookService interface {
	ProcessIntegrationResult(ctx context.Context, req CDNIntegrationResultRequest) error
	ProcessAcknowledgment(ctx context.Context, req CDNAcknowledgmentRequest) error
}

// TaskCompleter defines the task completion interface needed from the workflow
// manager to advance tasks parked while ASYCUDA holds the next move.
//
// stepID is the parked task's active_step_id, read with the task itself, so a
// callback only ever completes the step it was matched to.
type TaskCompleter interface {
	CompleteTaskStep(ctx context.Context, taskID, stepID string, payload map[string]any) error
}

type cdnWebhookService struct {
	repo        DispatchNoteRepository
	tasks       parkedtask.Tasks
	taskManager TaskCompleter
}

// NewCDNWebhookService creates a new CDNWebhookService. tasks (core's task
// store) and taskManager carry the workflow side: the callbacks resume the task
// parked by the workflow the dispatch note recorded. Both may be nil, in which
// case callbacks are recorded but no workflow is advanced.
func NewCDNWebhookService(repo DispatchNoteRepository, tasks parkedtask.Tasks, taskManager TaskCompleter) CDNWebhookService {
	return &cdnWebhookService{repo: repo, tasks: tasks, taskManager: taskManager}
}

func (s *cdnWebhookService) ProcessIntegrationResult(ctx context.Context, req CDNIntegrationResultRequest) error {
	slog.InfoContext(ctx, "processing CDN integration result",
		"edge_id", req.Payload.EdgeID,
		"integrated", req.Payload.Integrated,
		"event", req.Event,
	)

	note, err := s.repo.GetByEdgeID(ctx, req.Payload.EdgeID)
	if err != nil {
		return fmt.Errorf("failed to retrieve dispatch note by edgeId %s: %w", req.Payload.EdgeID, err)
	}

	// The dispatch step records the note as soon as SLC Edge accepts it, before
	// SLC Edge can call back, so a result for an edgeId with no row is early, not
	// new: it is retried rather than recorded without its workflow.
	if note == nil {
		return fmt.Errorf("edgeId %s: %w", req.Payload.EdgeID, ErrDispatchNoteNotRecorded)
	}

	// A note already in a terminal state is not recorded again — but the parked
	// task still has to be released. The record and the workflow advance
	// separately, so a delivery that lands while the task is not yet parked (or
	// whose first delivery failed after the write) would otherwise leave the
	// trader waiting on a note Customs has already answered.
	if note.Status == DispatchNoteStatusIntegrated || note.Status == DispatchNoteStatusAcknowledged {
		slog.InfoContext(ctx, "dispatch note already recorded; releasing any task still parked on it",
			"edge_id", req.Payload.EdgeID, "status", note.Status)
		return s.resumeIntegrationWait(ctx, note, storedResult(req, note), true)
	}
	if !req.Payload.Integrated && note.Status == DispatchNoteStatusFailed {
		slog.InfoContext(ctx, "dispatch note already failed; releasing any task still parked on it", "edge_id", req.Payload.EdgeID)
		return s.resumeIntegrationWait(ctx, note, req, true)
	}

	// The trader's CDN task is parked on the integration wait; releasing it
	// completes the dispatch note step and opens the acknowledgment wait. It is
	// released before the result is recorded: once recorded, SLC Edge's retry is
	// a redelivery, so a lookup that fails or a task not parked yet must leave
	// the note as it was for the retry to work.
	if err := s.resumeIntegrationWait(ctx, note, req, false); err != nil {
		return err
	}

	if req.Payload.Integrated {
		note.Status = DispatchNoteStatusIntegrated
		note.CDNYear = req.Payload.CDNRef.Year
		note.CDNOffice = req.Payload.CDNRef.Office
		note.CDNSerial = req.Payload.CDNRef.Serial
		note.CDNNumber = req.Payload.CDNRef.Number

		if err := s.repo.Update(ctx, note); err != nil {
			return fmt.Errorf("failed to record dispatch note as INTEGRATED: %w", err)
		}

		slog.InfoContext(ctx, "dispatch note integrated successfully",
			"edge_id", req.Payload.EdgeID,
			"cdn_ref", req.Payload.CDNRef,
		)
	} else {
		note.Status = DispatchNoteStatusFailed
		note.Errors = req.Payload.Errors
		if err := s.repo.Update(ctx, note); err != nil {
			return fmt.Errorf("failed to record dispatch note as FAILED: %w", err)
		}

		slog.WarnContext(ctx, "dispatch note integration failed",
			"edge_id", req.Payload.EdgeID,
			"errors", string(req.Payload.Errors),
		)
	}
	return nil
}

// storedResult re-points an already-recorded result at the reference actually on
// the note. The row is the authority once written: replaying a callback that
// carries a different cdnRef must not hand the workflow a reference that no
// longer matches what was stored, or the acknowledgment could never correlate.
func storedResult(req CDNIntegrationResultRequest, note *DispatchNote) CDNIntegrationResultRequest {
	req.Payload.Integrated = note.Status == DispatchNoteStatusIntegrated ||
		note.Status == DispatchNoteStatusAcknowledged
	req.Payload.CDNRef = DocumentReference{
		Office: note.CDNOffice,
		Year:   note.CDNYear,
		Serial: note.CDNSerial,
		Number: note.CDNNumber,
	}
	return req
}

func (s *cdnWebhookService) ProcessAcknowledgment(ctx context.Context, req CDNAcknowledgmentRequest) error {
	ref := req.Payload.CDNRef

	slog.InfoContext(ctx, "processing CDN acknowledgment",
		"event", req.Event,
		"cdn_ref", ref,
	)

	note, err := s.repo.GetByCDNRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to retrieve dispatch note by cdnRef: %w", err)
	}
	if note == nil {
		slog.WarnContext(ctx, "no dispatch note found for cdnRef",
			"year", ref.Year,
			"office", ref.Office,
			"serial", ref.Serial,
			"number", ref.Number,
		)
		return fmt.Errorf("cdnRef %v: %w", ref, ErrDispatchNoteNotFoundByCDNRef)
	}

	if note.Status == DispatchNoteStatusAcknowledged {
		slog.InfoContext(ctx, "dispatch note already acknowledged; releasing any task still parked on it", "cdn_ref", ref)
		return s.resumeAcknowledgmentWait(ctx, note, true)
	}

	if note.Status != DispatchNoteStatusIntegrated {
		return fmt.Errorf("invalid state transition: cannot acknowledge dispatch note in status %s", note.Status)
	}

	// Released before the acknowledgment is recorded, for the same reason as the
	// integration wait: a recorded acknowledgment makes SLC Edge's retry a
	// redelivery.
	if err := s.resumeAcknowledgmentWait(ctx, note, false); err != nil {
		return err
	}

	note.Status = DispatchNoteStatusAcknowledged
	if err := s.repo.Update(ctx, note); err != nil {
		return fmt.Errorf("failed to update dispatch note to ACKNOWLEDGED: %w", err)
	}

	slog.InfoContext(ctx, "dispatch note acknowledged",
		"id", note.ID,
		"cdn_ref", ref,
	)
	return nil
}
