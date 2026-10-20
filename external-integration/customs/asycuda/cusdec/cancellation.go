package cusdec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenNSW/core/taskflow/store"
)

// EventCusdecCancelled is the §6.5.5 Declaration Cancelled notification: Customs
// cancelled a registered declaration in ASYCUDA World. Like an amendment it can
// arrive at any point after registration.
const EventCusdecCancelled = "CUSDEC_CANCELLED"

// CancelledFlowTemplateID is the flow a cancellation opens on the consignment, to
// tell the trader why its export flow ended.
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

// Workflows ends the workflow a cancelled declaration was dispatched from.
// core's workflow manager satisfies it.
type Workflows interface {
	// RootWorkflowID returns the top-level workflow workflowID runs under.
	RootWorkflowID(ctx context.Context, workflowID string) (string, error)
	// TerminateWorkflow ends workflowID and the child workflows it started.
	TerminateWorkflow(ctx context.Context, workflowID, reason string) error
}

// ProcessCancellation ends the consignment's export flow when Customs cancels
// its declaration: the flow the declaration was dispatched from is terminated,
// with its branches, every task of it still open is cancelled, the flow is
// recorded as ended, and a notice flow tells the trader why.
//
// Each step can be repeated, and the declaration is marked cancelled only once
// all have run, so a delivery that failed part-way is completed by SLC Edge's
// retry (§2). One that arrives after that is acknowledged without action.
func (s *webhookService) ProcessCancellation(ctx context.Context, req CusdecCancelledRequest) error {
	ref := req.Payload.CusdecRef
	decl, err := s.repo.GetByCusdecRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to retrieve CusDec declaration by ref: %w", err)
	}
	if decl == nil {
		return fmt.Errorf("cusdecRef %v: %w", ref, ErrCusdecNotFoundByRef)
	}
	if decl.Status == CusdecStatusCancelled {
		return ErrDuplicateEvent
	}
	if decl.CaseID == "" || decl.WorkflowID == "" {
		return fmt.Errorf("cusdecRef %v: %w", ref, ErrCaseNotRecorded)
	}
	if s.workflows == nil || s.flows == nil || s.tasks == nil || s.taskManager == nil {
		return errors.New("cusdec: no workflow wiring to end the consignment with")
	}

	cusdecNumber := formatCusdecRef(ref)
	reason := fmt.Sprintf("Customs cancelled declaration %s", cusdecNumber)

	root, err := s.workflows.RootWorkflowID(ctx, decl.WorkflowID)
	if err != nil {
		return fmt.Errorf("cusdec: find the flow declaration %s belongs to: %w", cusdecNumber, err)
	}
	if err := s.workflows.TerminateWorkflow(ctx, root, reason); err != nil {
		return fmt.Errorf("cusdec: end flow %s: %w", root, err)
	}
	if err := s.cancelOpenTasks(ctx, decl.CaseID, root, reason); err != nil {
		return err
	}
	if err := s.flows.EndFlow(ctx, root); err != nil {
		return fmt.Errorf("cusdec: record flow %s as ended: %w", root, err)
	}

	vars := map[string]any{
		"consignmentId": decl.CaseID,
		"cusdec_number": cusdecNumber,
		"cancelled_at":  req.ProcessedAt.UTC().Format(time.RFC3339),
	}
	if err := s.flows.StartFlow(ctx, decl.CaseID, CancelledFlowTemplateID, cancellationWorkflowID(decl.CaseID, ref), vars); err != nil {
		return fmt.Errorf("cusdec: open the cancellation notice: %w", err)
	}

	decl.Status = CusdecStatusCancelled
	if err := s.repo.Update(ctx, decl); err != nil {
		return fmt.Errorf("failed to record CusDec declaration %s as cancelled: %w", cusdecNumber, err)
	}
	slog.InfoContext(ctx, "CusDec cancelled: ended the consignment's flow",
		"cusdec_ref", ref, "case_id", decl.CaseID, "workflow_id", root)
	return nil
}

// cancelOpenTasks cancels every task of the case that has not ended and runs
// under the terminated flow root. Terminating a flow does not end the tasks it
// started, which run as workflows of their own; tasks of the case's other flows,
// such as an amendment notice, are left as they are.
func (s *webhookService) cancelOpenTasks(ctx context.Context, caseID, root, reason string) error {
	for _, rec := range s.tasks.GetAllTasks(ctx, caseID) {
		if rec.State == store.StateCompleted || rec.State == store.StateCancelled {
			continue
		}
		taskRoot, err := s.workflows.RootWorkflowID(ctx, rec.ParentWorkflowID)
		if err != nil {
			return fmt.Errorf("cusdec: find the flow task %s belongs to: %w", rec.TaskID, err)
		}
		if taskRoot != root {
			continue
		}
		if err := s.taskManager.CancelTask(ctx, rec.TaskID, reason); err != nil {
			return fmt.Errorf("cusdec: cancel task %s: %w", rec.TaskID, err)
		}
	}
	return nil
}

// cancellationWorkflowID names the notice flow a cancellation opens. A
// declaration is cancelled once, so every delivery of it names the same flow.
func cancellationWorkflowID(caseID string, ref DocumentReference) string {
	sum := sha256.Sum256([]byte(formatCusdecRef(ref)))
	return caseID + ":" + CancelledFlowTemplateID + ":" + hex.EncodeToString(sum[:8])
}
