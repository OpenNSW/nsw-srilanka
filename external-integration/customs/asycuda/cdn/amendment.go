package cdn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// EventCDNAmended is the §7.6 CDN Amended notification: Customs amended a
// registered dispatch note in ASYCUDA World. It can arrive at any point after
// registration, whatever the note's workflow is doing.
const EventCDNAmended = "CDN_AMENDED"

// AmendedFlowTemplateID is the flow a dispatch note amendment opens on the
// consignment, for the trader to see the note as amended.
const AmendedFlowTemplateID = "customs-cdn-amended"

// CDNAmendedRequest is the §7.6 notification, in the notification envelope
// (§4.6).
type CDNAmendedRequest struct {
	EventType   string         `json:"eventType"`
	ProcessedAt time.Time      `json:"processedAt"`
	Payload     amendedPayload `json:"payload"`
}

type amendedPayload struct {
	// CDNRef is the registered reference (§4.1) the amendment is correlated by.
	CDNRef DocumentReference `json:"cdnRef"`
	// CDN is the full dispatch note after the amendment, as Annex B, with its
	// linked declarations. Kept as sent: the flow shows it, nothing here reads it.
	CDN json.RawMessage `json:"cdn"`
	// OperationTime is when the amendment was made in ASYCUDA World.
	OperationTime string `json:"operationTime"`
}

// Validate checks the fields §7.6 requires are present.
func (r CDNAmendedRequest) Validate() error {
	if r.EventType != EventCDNAmended {
		return fmt.Errorf("eventType must be %s", EventCDNAmended)
	}
	if !r.Payload.CDNRef.IsValid() {
		return errors.New("payload.cdnRef must be fully populated")
	}
	if c := bytes.TrimSpace(r.Payload.CDN); len(c) == 0 || c[0] != '{' {
		return errors.New("payload.cdn must be the amended dispatch note object")
	}
	if strings.TrimSpace(r.Payload.OperationTime) == "" {
		return errors.New("payload.operationTime is required")
	}
	return nil
}

// CaseFlows starts flows on a case. Starting again with the same workflowID is
// a no-op, so a redelivered notification opens nothing twice.
type CaseFlows interface {
	StartFlow(ctx context.Context, caseID, templateID, workflowID string, vars map[string]any) error
}

// ProcessAmendment opens the amendment flow on the dispatch note's case, seeded
// with the amended note, so the trader sees what Customs changed. The note's own
// workflow is not touched: an amendment can arrive while it waits on the
// acknowledgment, or after it has finished.
func (s *cdnWebhookService) ProcessAmendment(ctx context.Context, req CDNAmendedRequest) error {
	ref := req.Payload.CDNRef
	note, err := s.repo.GetByCDNRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to retrieve dispatch note by cdnRef: %w", err)
	}
	if note == nil {
		return fmt.Errorf("cdnRef %v: %w", ref, ErrDispatchNoteNotFoundByCDNRef)
	}
	if note.CaseID == "" {
		return fmt.Errorf("cdnRef %v: %w", ref, ErrCaseNotRecorded)
	}
	if s.flows == nil {
		return errors.New("cdn: no case flows to open the amendment on")
	}

	var amended map[string]any
	if err := json.Unmarshal(req.Payload.CDN, &amended); err != nil {
		return fmt.Errorf("cdn: decode amended dispatch note: %w", err)
	}
	operationTime := strings.TrimSpace(req.Payload.OperationTime)
	vars := map[string]any{
		"consignmentId": note.CaseID,
		"cdn_number":    formatCDNRef(ref),
		"amended_at":    operationTime,
		"cdn":           amended,
	}

	workflowID := amendmentWorkflowID(note.CaseID, ref, operationTime)
	if err := s.flows.StartFlow(ctx, note.CaseID, AmendedFlowTemplateID, workflowID, vars); err != nil {
		return fmt.Errorf("cdn: open the amendment flow: %w", err)
	}
	slog.InfoContext(ctx, "opened CDN amendment flow",
		"cdn_ref", ref, "case_id", note.CaseID, "workflow_id", workflowID, "operation_time", operationTime)
	return nil
}

// amendmentWorkflowID names the flow one amendment opens. SLC Edge retries a
// notification (§2), and every delivery of one amendment carries the same
// reference and operation time, so it names the same flow and is started once.
// A later amendment of the same note has a new operation time and opens a flow
// of its own.
func amendmentWorkflowID(caseID string, ref DocumentReference, operationTime string) string {
	sum := sha256.Sum256([]byte(formatCDNRef(ref) + "\x00" + operationTime))
	return caseID + ":" + AmendedFlowTemplateID + ":" + hex.EncodeToString(sum[:8])
}

// formatCDNRef writes a registered reference the way the workflow shows it:
// office/year/serial/number.
func formatCDNRef(ref DocumentReference) string {
	return fmt.Sprintf("%s/%s/%s/%d", ref.Office, ref.Year, ref.Serial, ref.Number)
}
