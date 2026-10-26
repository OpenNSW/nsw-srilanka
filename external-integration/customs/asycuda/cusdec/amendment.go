package cusdec

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

// EventCusdecAmended is the §6.5.4 Declaration Amended notification: Customs
// amended a registered declaration in ASYCUDA World. It can arrive at any point
// after registration, whatever the declaration's workflow is doing.
const EventCusdecAmended = "CUSDEC_AMENDED"

// AmendedFlowTemplateID is the flow a declaration amendment opens on the
// consignment, for the trader to see the declaration as amended and its duties.
const AmendedFlowTemplateID = "customs-cusdec-amended"

// CusdecAmendedRequest is the §6.5.4 notification, in the notification envelope
// (§4.6).
type CusdecAmendedRequest struct {
	EventType   string         `json:"eventType"`
	ProcessedAt time.Time      `json:"processedAt"`
	Payload     amendedPayload `json:"payload"`
}

type amendedPayload struct {
	// CusdecRef is the registered reference (§4.1) the amendment is correlated by.
	CusdecRef DocumentReference `json:"cusDecRef"`
	// Cusdec is the full declaration after the amendment, as Annex A. Kept as
	// sent: the flow shows it, nothing here reads it.
	Cusdec json.RawMessage `json:"cusDec"`
	// Duties are the duties and taxes as reassessed (as on §6.6).
	Duties verifyDuties `json:"duties"`
	// OperationTime is when the amendment was made in ASYCUDA World.
	OperationTime string `json:"operationTime"`
}

// Validate checks the fields §6.5.4 requires are present.
func (r CusdecAmendedRequest) Validate() error {
	if r.EventType != EventCusdecAmended {
		return fmt.Errorf("eventType must be %s", EventCusdecAmended)
	}
	if !r.Payload.CusdecRef.IsValid() {
		return errors.New("payload.cusDecRef must be fully populated")
	}
	if c := bytes.TrimSpace(r.Payload.Cusdec); len(c) == 0 || c[0] != '{' {
		return errors.New("payload.cusDec must be the amended declaration object")
	}
	if strings.TrimSpace(r.Payload.OperationTime) == "" {
		return errors.New("payload.operationTime is required")
	}
	return nil
}

// CaseFlows starts a flow on a case. Starting again with the same workflowID is
// a no-op, so a redelivered notification opens nothing twice.
type CaseFlows interface {
	StartFlow(ctx context.Context, caseID, templateID, workflowID string, vars map[string]any) error
}

// ProcessAmendment opens the amendment flow on the declaration's case, seeded
// with the amended declaration and its duties, so the trader sees what Customs
// changed. The declaration's own workflow is not touched: an amendment can
// arrive while it waits on payment or warranting, or after it has finished.
func (s *webhookService) ProcessAmendment(ctx context.Context, req CusdecAmendedRequest) error {
	ref := req.Payload.CusdecRef
	decl, err := s.repo.GetByCusdecRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to retrieve CusDec declaration by ref: %w", err)
	}
	if decl == nil {
		return fmt.Errorf("cusDecRef %v: %w", ref, ErrCusdecNotFoundByRef)
	}
	if decl.CaseID == "" {
		return fmt.Errorf("cusDecRef %v: %w", ref, ErrCaseNotRecorded)
	}
	if s.flows == nil {
		return errors.New("cusdec: no case flows to open the amendment on")
	}

	var cusdec map[string]any
	if err := json.Unmarshal(req.Payload.Cusdec, &cusdec); err != nil {
		return fmt.Errorf("cusdec: decode amended declaration: %w", err)
	}
	operationTime := strings.TrimSpace(req.Payload.OperationTime)
	vars := map[string]any{
		"consignmentId": decl.CaseID,
		"cusdec_number": formatCusdecRef(ref),
		"amended_at":    operationTime,
		"cusdec":        cusdec,
	}
	if duties, ok := dutiesForTask(req.Payload.Duties); ok {
		vars["duties"] = duties
	}

	workflowID := amendmentWorkflowID(decl.CaseID, ref, operationTime)
	if err := s.flows.StartFlow(ctx, decl.CaseID, AmendedFlowTemplateID, workflowID, vars); err != nil {
		return fmt.Errorf("cusdec: open the amendment flow: %w", err)
	}
	slog.InfoContext(ctx, "opened CusDec amendment flow",
		"cusdec_ref", ref, "case_id", decl.CaseID, "workflow_id", workflowID, "operation_time", operationTime)
	return nil
}

// amendmentWorkflowID names the flow one amendment opens. SLC Edge retries a
// notification (§2), and every delivery of one amendment carries the same
// reference and operation time, so it names the same flow and is started once.
// A later amendment of the same declaration has a new operation time and opens
// a flow of its own.
func amendmentWorkflowID(caseID string, ref DocumentReference, operationTime string) string {
	sum := sha256.Sum256([]byte(formatCusdecRef(ref) + "\x00" + operationTime))
	return caseID + ":" + AmendedFlowTemplateID + ":" + hex.EncodeToString(sum[:8])
}

// formatCusdecRef writes a registered reference the way the workflow shows it:
// office/year/serial/number.
func formatCusdecRef(ref DocumentReference) string {
	return fmt.Sprintf("%s/%s/%s/%d", ref.Office, ref.Year, ref.Serial, ref.Number)
}
