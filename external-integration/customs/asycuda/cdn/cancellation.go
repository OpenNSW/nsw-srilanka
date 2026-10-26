package cdn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// EventCDNCancelled is the §7.7 CDN Cancelled notification: Customs cancelled a
// registered dispatch note in ASYCUDA World.
const EventCDNCancelled = "CDN_CANCELLED"

// CDNCancelledRequest is the §7.7 notification, in the notification envelope
// (§4.6).
type CDNCancelledRequest struct {
	EventType   string           `json:"eventType"`
	ProcessedAt time.Time        `json:"processedAt"`
	Payload     cancelledPayload `json:"payload"`
}

type cancelledPayload struct {
	// CDNRef is the registered reference (§4.1) of the cancelled dispatch note.
	CDNRef DocumentReference `json:"cdnRef"`
}

// Validate checks the fields §7.7 requires are present.
func (r CDNCancelledRequest) Validate() error {
	if r.EventType != EventCDNCancelled {
		return fmt.Errorf("eventType must be %s", EventCDNCancelled)
	}
	if r.ProcessedAt.IsZero() {
		return errors.New("processedAt is required")
	}
	if !r.Payload.CDNRef.IsValid() {
		return errors.New("payload.cdnRef must be fully populated")
	}
	return nil
}

// ProcessCancellation ends the container's dispatch note branch when Customs
// cancels its note. The trader cannot raise another note for that container, so
// the acknowledgment wait is released as cancelled and the branch finishes; the
// consignment's other containers, and the declaration, carry on.
//
// The wait is released before the note is marked cancelled, as an acknowledgment
// does: once the note is cancelled, SLC Edge's retry is a redelivery, so a
// lookup that fails or a wait not parked yet must leave the note as it was for
// the retry (§2) to release it.
func (s *cdnWebhookService) ProcessCancellation(ctx context.Context, req CDNCancelledRequest) error {
	ref := req.Payload.CDNRef
	note, err := s.repo.GetByCDNRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to retrieve dispatch note by cdnRef: %w", err)
	}
	if note == nil {
		return fmt.Errorf("cdnRef %v: %w", ref, ErrDispatchNoteNotFoundByCDNRef)
	}

	switch note.Status {
	case DispatchNoteStatusCancelled:
		slog.InfoContext(ctx, "dispatch note already cancelled; releasing any task still parked on it", "cdn_ref", ref)
		return s.resumeCancelledWait(ctx, note, true)
	case DispatchNoteStatusAcknowledged:
		// The branch finished when the note was acknowledged, so there is no wait
		// left to release; the note is still recorded as cancelled.
		slog.WarnContext(ctx, "acknowledged dispatch note cancelled; its branch has already finished", "cdn_ref", ref)
	case DispatchNoteStatusIntegrated:
	default:
		return fmt.Errorf("invalid state transition: cannot cancel dispatch note in status %s", note.Status)
	}

	if note.Status == DispatchNoteStatusIntegrated {
		if err := s.resumeCancelledWait(ctx, note, false); err != nil {
			return err
		}
	}

	note.Status = DispatchNoteStatusCancelled
	if err := s.repo.Update(ctx, note); err != nil {
		return fmt.Errorf("failed to update dispatch note to CANCELLED: %w", err)
	}
	slog.InfoContext(ctx, "dispatch note cancelled", "id", note.ID, "cdn_ref", ref)
	return nil
}
