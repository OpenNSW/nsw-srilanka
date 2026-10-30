package cusdec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// EventCusdecCancelled is the §6.5.5 Declaration Cancelled notification: Customs
// cancelled a registered declaration in ASYCUDA World. Like an amendment it can
// arrive at any point after registration.
const EventCusdecCancelled = "CUSDEC_CANCELLED"

// CancelledFlowTemplateID is the flow a cancellation opens on the consignment, to
// tell the trader the declaration was cancelled.
const CancelledFlowTemplateID = "customs-cusdec-cancelled"

// CusdecCancelledRequest is the §6.5.5 notification, in the notification
// envelope (§4.6).
type CusdecCancelledRequest struct {
	EventType   string           `json:"eventType"`
	ProcessedAt time.Time        `json:"processedAt"`
	Payload     cancelledPayload `json:"payload"`
}

type cancelledPayload struct {
	// CusdecRef is the registered reference (§4.1) of the cancelled declaration.
	CusdecRef DocumentReference `json:"cusdecRef"`
}

// Validate checks the fields §6.5.5 requires are present.
func (r CusdecCancelledRequest) Validate() error {
	if r.EventType != EventCusdecCancelled {
		return fmt.Errorf("eventType must be %s", EventCusdecCancelled)
	}
	if r.ProcessedAt.IsZero() {
		return errors.New("processedAt is required")
	}
	if !r.Payload.CusdecRef.IsValid() {
		return errors.New("payload.cusdecRef must be fully populated")
	}
	return nil
}

// ProcessCancellation tells the trader Customs cancelled the declaration: it
// opens a notice flow on the consignment saying the declaration was cancelled
// and can no longer be proceeded with, which the trader acknowledges. Nothing
// is cancelled here: the consignment's flows and tasks, and the declaration's
// own status, are left as they are, so Customs' later notifications for the
// declaration are still applied as before.
//
// The notice names the same flow on every delivery of the cancellation, so SLC
// Edge's retry (§2) opens it once.
func (s *webhookService) ProcessCancellation(ctx context.Context, req CusdecCancelledRequest) error {
	ref := req.Payload.CusdecRef
	decl, err := s.repo.GetByCusdecRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to retrieve CusDec declaration by ref: %w", err)
	}
	if decl == nil {
		return fmt.Errorf("cusdecRef %v: %w", ref, ErrCusdecNotFoundByRef)
	}
	if decl.CaseID == "" {
		return fmt.Errorf("cusdecRef %v: %w", ref, ErrCaseNotRecorded)
	}
	if s.flows == nil {
		return errors.New("cusdec: no case flows to open the cancellation notice on")
	}

	cusdecNumber := formatCusdecRef(ref)
	vars := map[string]any{
		"consignmentId": decl.CaseID,
		"cusdec_number": cusdecNumber,
		"cancelled_at":  req.ProcessedAt.UTC().Format(time.RFC3339),
	}
	if err := s.flows.StartFlow(ctx, decl.CaseID, CancelledFlowTemplateID, cancellationWorkflowID(decl.CaseID, ref), vars); err != nil {
		return fmt.Errorf("cusdec: open the cancellation notice: %w", err)
	}
	slog.InfoContext(ctx, "CusDec cancelled: opened the cancellation notice",
		"cusdec_ref", ref, "case_id", decl.CaseID)
	return nil
}

// cancellationWorkflowID names the notice flow a cancellation opens. A
// declaration is cancelled once, so every delivery of it names the same flow.
func cancellationWorkflowID(caseID string, ref DocumentReference) string {
	sum := sha256.Sum256([]byte(formatCusdecRef(ref)))
	return caseID + ":" + CancelledFlowTemplateID + ":" + hex.EncodeToString(sum[:8])
}
