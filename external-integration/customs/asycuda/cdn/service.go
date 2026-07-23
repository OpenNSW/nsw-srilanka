package cdn

import (
	"context"
	"fmt"
	"log/slog"
)

// CDNWebhookService defines the use-case layer for processing asynchronous
// ASYCUDA callbacks related to Cargo Dispatch Notes.
type CDNWebhookService interface {
	ProcessIntegrationResult(ctx context.Context, req CDNIntegrationResultRequest) error
	ProcessAcknowledgment(ctx context.Context, req CDNAcknowledgmentRequest) error
}

type cdnWebhookService struct {
	repo DispatchNoteRepository
}

// NewCDNWebhookService creates a new CDNWebhookService backed by the given repository.
func NewCDNWebhookService(repo DispatchNoteRepository) CDNWebhookService {
	return &cdnWebhookService{repo: repo}
}

func (s *cdnWebhookService) ProcessIntegrationResult(ctx context.Context, req CDNIntegrationResultRequest) error {
	slog.InfoContext(ctx, "processing CDN integration result",
		"edg_id", req.EdgID,
		"integrated", req.Integrated,
		"event", req.Event,
	)

	note, err := s.repo.GetByEdgID(ctx, req.EdgID)
	if err != nil {
		return fmt.Errorf("failed to retrieve dispatch note by edgId %s: %w", req.EdgID, err)
	}
	if note == nil {
		slog.WarnContext(ctx, "no dispatch note found for edgId", "edg_id", req.EdgID)
		return fmt.Errorf("edgId %s: %w", req.EdgID, ErrDispatchNoteNotFoundByEdgID)
	}

	if note.Status == DispatchNoteStatusIntegrated || note.Status == DispatchNoteStatusAcknowledged {
		slog.InfoContext(ctx, "dispatch note already processed, ignoring integration result", "edg_id", req.EdgID, "status", note.Status)
		return nil
	}

	if req.Integrated {
		note.Status = DispatchNoteStatusIntegrated
		note.CDNYear = req.Payload.CDNRef.Year
		note.CDNOffice = req.Payload.CDNRef.Office
		note.CDNSerial = req.Payload.CDNRef.Serial
		note.CDNNumber = req.Payload.CDNRef.Number

		if err := s.repo.Update(ctx, note); err != nil {
			return fmt.Errorf("failed to update dispatch note to INTEGRATED: %w", err)
		}

		slog.InfoContext(ctx, "dispatch note integrated successfully",
			"edg_id", req.EdgID,
			"cdn_ref", req.Payload.CDNRef,
		)
	} else {
		if note.Status == DispatchNoteStatusFailed {
			slog.InfoContext(ctx, "dispatch note already failed, ignoring duplicate callback", "edg_id", req.EdgID)
			return nil
		}

		note.Status = DispatchNoteStatusFailed
		note.Errors = req.Errors
		if err := s.repo.Update(ctx, note); err != nil {
			return fmt.Errorf("failed to update dispatch note to FAILED: %w", err)
		}

		slog.WarnContext(ctx, "dispatch note integration failed",
			"edg_id", req.EdgID,
			"errors", string(req.Errors),
		)
	}

	return nil
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
		slog.InfoContext(ctx, "dispatch note already acknowledged, ignoring duplicate callback", "cdn_ref", ref)
		return nil
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

	return nil
}
