package cdn

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CDNWebhookService defines the use-case layer for processing asynchronous
// ASYCUDA callbacks related to Cargo Dispatch Notes.
type CDNWebhookService interface {
	ProcessIntegrationResult(ctx context.Context, req CDNIntegrationResultRequest) error
	ProcessAcknowledgment(ctx context.Context, req CDNAcknowledgmentRequest) error
}

// TaskCompleter defines the task completion interface needed from the workflow
// manager to advance tasks parked while ASYCUDA holds the next move.
type TaskCompleter interface {
	CompleteTaskStep(ctx context.Context, taskID string, payload map[string]any) error
}

type cdnWebhookService struct {
	repo        DispatchNoteRepository
	db          *gorm.DB
	taskManager TaskCompleter
}

// NewCDNWebhookService creates a new CDNWebhookService. db and taskManager carry
// the workflow side: the callbacks resume the task parked against the dispatch
// note. Both may be nil, in which case callbacks are recorded but no workflow is
// advanced.
func NewCDNWebhookService(repo DispatchNoteRepository, db *gorm.DB, taskManager TaskCompleter) CDNWebhookService {
	return &cdnWebhookService{repo: repo, db: db, taskManager: taskManager}
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

	// The §7.1 acknowledgement is recorded in the workflow, not in this table:
	// the dispatch step is fire-and-forget from the plugin's point of view, so
	// the first this service hears of an edgeId is the integration result. A row
	// is therefore created on demand rather than looked up, which is also what
	// the CusDec side does with its declarations.
	isNew := note == nil
	if isNew {
		note = &DispatchNote{
			ID:     uuid.NewString(),
			EdgeID: req.Payload.EdgeID,
			Status: DispatchNoteStatusSubmitted,
		}
	}

	// A note already in a terminal state is not recorded again — but the parked
	// task still has to be released. The record and the workflow advance
	// separately, so a delivery that lands while the task is not yet parked (or
	// whose first delivery failed after the write) would otherwise leave the
	// trader waiting on a note Customs has already answered.
	if note.Status == DispatchNoteStatusIntegrated || note.Status == DispatchNoteStatusAcknowledged {
		slog.InfoContext(ctx, "dispatch note already recorded; releasing any task still parked on it",
			"edge_id", req.Payload.EdgeID, "status", note.Status)
		return s.resumeIntegrationWait(ctx, storedResult(req, note), true)
	}

	save := s.repo.Update
	if isNew {
		save = s.repo.Create
	}

	if req.Payload.Integrated {
		note.Status = DispatchNoteStatusIntegrated
		note.CDNYear = req.Payload.CDNRef.Year
		note.CDNOffice = req.Payload.CDNRef.Office
		note.CDNSerial = req.Payload.CDNRef.Serial
		note.CDNNumber = req.Payload.CDNRef.Number

		if err := save(ctx, note); err != nil {
			return fmt.Errorf("failed to record dispatch note as INTEGRATED: %w", err)
		}

		slog.InfoContext(ctx, "dispatch note integrated successfully",
			"edge_id", req.Payload.EdgeID,
			"cdn_ref", req.Payload.CDNRef,
		)
	} else {
		if note.Status == DispatchNoteStatusFailed {
			slog.InfoContext(ctx, "dispatch note already failed; releasing any task still parked on it", "edge_id", req.Payload.EdgeID)
			return s.resumeIntegrationWait(ctx, req, true)
		}

		note.Status = DispatchNoteStatusFailed
		note.Errors = req.Payload.Errors
		if err := save(ctx, note); err != nil {
			return fmt.Errorf("failed to record dispatch note as FAILED: %w", err)
		}

		slog.WarnContext(ctx, "dispatch note integration failed",
			"edge_id", req.Payload.EdgeID,
			"errors", string(req.Payload.Errors),
		)
	}

	// The trader's CDN task is parked on the integration wait; releasing it
	// completes the dispatch note step and opens the acknowledgment wait.
	return s.resumeIntegrationWait(ctx, req, false)
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

	note.Status = DispatchNoteStatusAcknowledged
	if err := s.repo.Update(ctx, note); err != nil {
		return fmt.Errorf("failed to update dispatch note to ACKNOWLEDGED: %w", err)
	}

	slog.InfoContext(ctx, "dispatch note acknowledged",
		"id", note.ID,
		"cdn_ref", ref,
	)

	return s.resumeAcknowledgmentWait(ctx, note, false)
}
