package cusdec

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/parkedtask"
)

// WebhookService defines the use-case layer for processing asynchronous
// ASYCUDA callbacks related to Customs Declarations.
type WebhookService interface {
	ProcessIntegrationResult(ctx context.Context, req CusdecIntegrationResultRequest) error
	ProcessEvent(ctx context.Context, req CusdecEventRequest) error
}

// TaskCompleter defines the task completion interface needed from the workflow
// manager to advance suspended tasks.
//
// stepID is the parked task's active_step_id, read with the task itself, so a
// callback only ever completes the step it was matched to.
type TaskCompleter interface {
	CompleteTaskStep(ctx context.Context, taskID, stepID string, payload map[string]any) error
}

type webhookService struct {
	repo        DeclarationRepository
	tasks       parkedtask.Tasks
	taskManager TaskCompleter
}

// NewWebhookService creates a new WebhookService. tasks is core's task store:
// a callback resumes the task the declaration's workflow parked, found by the
// case and workflow the declaration recorded when it was dispatched.
func NewWebhookService(repo DeclarationRepository, tasks parkedtask.Tasks, taskManager TaskCompleter) WebhookService {
	return &webhookService{
		repo:        repo,
		tasks:       tasks,
		taskManager: taskManager,
	}
}

// reviewTemplateID is the step the declaration flow parks on until the
// integration result (§6.2) arrives.
const reviewTemplateID = "customs-cusdec--external-review"

func (s *webhookService) ProcessIntegrationResult(ctx context.Context, req CusdecIntegrationResultRequest) error {
	slog.InfoContext(ctx, "processing CusDec integration result",
		"edge_id", req.EdgeID,
		"integrated", req.Integrated,
		"event", req.Event,
	)

	// §6.2 delivers one integration result per edgeId, and §2 has SLC Edge
	// retry it up to four times. A repeat therefore means our acknowledgement
	// was lost, not that anything changed: re-running would complete the review
	// task a second time and drive the workflow past a step the trader has
	// already been through. The edgeId is the correlator for the whole
	// round-trip (§2.1), so it is what duplicates are judged on.
	//
	// Any status past SUBMITTED means the result was already recorded: the
	// §6.5 events that move a declaration on to PAID, WARRANTED or RELEASED
	// only follow an integration. Recording the repeat would put such a
	// declaration back to INTEGRATED.
	decl, err := s.repo.GetByEdgeID(ctx, req.EdgeID)
	if err != nil {
		return fmt.Errorf("failed to retrieve CusDec declaration by edgeId %s: %w", req.EdgeID, err)
	}
	// The dispatch step records the declaration as soon as SLC Edge accepts it,
	// before SLC Edge can call back, so a result for an edgeId with no row is
	// early, not new: it is retried rather than recorded without its workflow.
	if decl == nil {
		return fmt.Errorf("edgeId %s: %w", req.EdgeID, ErrDeclarationNotRecorded)
	}
	if decl.Status != CusdecStatusSubmitted {
		slog.InfoContext(ctx, "integration result already processed, acknowledging without action",
			"edge_id", req.EdgeID, "status", decl.Status)
		return ErrDuplicateIntegrationResult
	}

	// The task is completed before the result is recorded, as events are: once
	// recorded, SLC Edge's retry is a duplicate, so a lookup that fails or a task
	// not parked yet must leave the declaration SUBMITTED for the retry to work.
	task, err := s.findReviewTask(ctx, decl, req)
	if err != nil {
		return err
	}
	if err := s.taskManager.CompleteTaskStep(ctx, task.TaskID, task.StepID, reviewPayload(req)); err != nil {
		slog.ErrorContext(ctx, "failed to complete external review task step", "task_id", task.TaskID, "error", err)
		return fmt.Errorf("complete the external review task %s: %w", task.TaskID, err)
	}

	if err := s.recordIntegrationResult(ctx, decl, req); err != nil {
		return err
	}

	slog.InfoContext(ctx, "successfully completed external review task step and advanced workflow",
		"edge_id", req.EdgeID,
		"integrated", req.Integrated,
	)
	return nil
}

func (s *webhookService) recordIntegrationResult(ctx context.Context, decl *CusdecDeclaration, req CusdecIntegrationResultRequest) error {
	if req.Integrated {
		decl.Status = CusdecStatusIntegrated
		decl.CusdecYear = req.Payload.CusdecRef.Year
		decl.CusdecOffice = req.Payload.CusdecRef.Office
		decl.CusdecSerial = req.Payload.CusdecRef.Serial
		decl.CusdecNumber = req.Payload.CusdecRef.Number
		decl.Errors = nil
	} else {
		decl.Status = CusdecStatusFailed
		decl.Errors = req.Errors
	}

	if err := s.repo.Update(ctx, decl); err != nil {
		return fmt.Errorf("failed to update CusDec declaration for edgeId %s: %w", decl.EdgeID, err)
	}
	return nil
}

// findReviewTask returns the review task the declaration's workflow parked.
// Neither error is acknowledged: the declaration is still SUBMITTED, so SLC
// Edge's retry of a 5xx (§2) finds the task once it can be read or has parked.
func (s *webhookService) findReviewTask(ctx context.Context, decl *CusdecDeclaration, req CusdecIntegrationResultRequest) (parkedtask.Task, error) {
	if decl.WorkflowID == "" {
		return parkedtask.Task{}, fmt.Errorf("edgeId %s: %w", req.EdgeID, ErrWorkflowNotFoundByEdgeID)
	}

	task, ok, err := parkedtask.Find(ctx, s.tasks, decl.CaseID, decl.WorkflowID, reviewTemplateID)
	if err != nil {
		return parkedtask.Task{}, fmt.Errorf("locate the external review task for workflow %s: %w", decl.WorkflowID, err)
	}
	if !ok {
		slog.ErrorContext(ctx, "failed to locate suspended external review task", "workflow_id", decl.WorkflowID)
		// Not a 404: the task may not have parked yet.
		return parkedtask.Task{}, fmt.Errorf("failed to locate suspended external review task for workflow %s", decl.WorkflowID)
	}
	return task, nil
}

// reviewPayload is what the review step is completed with: the assessment
// when ASYCUDA integrated the declaration, its errors when it did not.
func reviewPayload(req CusdecIntegrationResultRequest) map[string]any {
	var payload map[string]any
	if req.Integrated {
		formattedRef := fmt.Sprintf("%s/%s/%s/%d",
			req.Payload.CusdecRef.Office,
			req.Payload.CusdecRef.Year,
			req.Payload.CusdecRef.Serial,
			req.Payload.CusdecRef.Number,
		)
		payload = map[string]any{
			"__command":      "submit",
			"review_outcome": "approve",
			"cusdec_number":  formattedRef,
			// What the trader is asked to settle on the payment step that
			// follows -- ASYCUDA's own figure where it sent one.
			"amount_to_pay": amountToPay(req.Payload),
		}
		// The rest of the v1.9 assessment, passed on only when ASYCUDA sent it,
		// so the review step can state what was assessed and what is already
		// paid next to what is still due.
		if v := req.Payload.TotalAssessedAmount; v != nil {
			payload["total_assessed_amount"] = *v
		}
		if v := req.Payload.AmountPaid; v != nil {
			payload["amount_paid"] = *v
		}
		if duties, ok := dutiesForTask(req.Payload.Duties); ok {
			payload["duties"] = duties
		}
	} else {
		payload = map[string]any{
			"__command":        "submit",
			"review_outcome":   "needs_more_info",
			"rejection_reason": describeErrors(req.Errors),
		}
	}
	return payload
}

func (s *webhookService) ProcessEvent(ctx context.Context, req CusdecEventRequest) error {
	ref := req.Payload.CusdecRef
	slog.InfoContext(ctx, "processing CusDec event notification",
		"event", req.Event,
		"cusdec_ref", ref,
	)

	decl, err := s.repo.GetByCusdecRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to retrieve CusDec declaration by ref: %w", err)
	}
	if decl == nil {
		slog.WarnContext(ctx, "no CusDec declaration found for ref", "ref", ref)
		return fmt.Errorf("cusDecRef %v: %w", ref, ErrCusdecNotFoundByRef)
	}

	var taskTemplateID string
	var targetStatus CusdecStatus
	var payload map[string]any

	switch req.Event {
	case "PAYMENT_CONFIRMED":
		taskTemplateID = "customs-wait-payment"
		targetStatus = CusdecStatusPaid
		payload = map[string]any{
			"__command":      "submit",
			"payment_status": "PAID",
			// §6.5.1 reports what Customs actually took. Without these the
			// trader's receipt could only echo the figure we assessed at
			// integration, so a discrepancy would be invisible.
			"amount_paid":    req.Payload.AmountPaid,
			"currency":       req.Payload.Currency,
			"bank_reference": req.Payload.BankReference,
		}
	case "WARRANTING_COMPLETED":
		taskTemplateID = "customs-wait-warranting"
		targetStatus = CusdecStatusWarranted
		payload = map[string]any{
			"__command":         "submit",
			"warranting_status": "WARRANTED",
			// §6.5.2 carries only the reference; releaseOrderNo is sent by
			// some deployments and shown when present.
			"release_order_no": req.Payload.ReleaseOrderNo,
		}
	case "EXPORT_RELEASED":
		taskTemplateID = "customs-wait-export-release"
		targetStatus = CusdecStatusReleased
		payload = map[string]any{
			"__command":      "submit",
			"release_status": "RELEASED",
			// §6.5.3 names the sailing the consignment left on — the only
			// record the trader gets of it.
			"vessel_name":     req.Payload.VesselName,
			"voyage_no":       req.Payload.VoyageNo,
			"port_of_loading": req.Payload.PortOfLoading,
		}
	default:
		return fmt.Errorf("unsupported CusDec event type: %s", req.Event)
	}

	// §6.5 events are correlated by cusdecRef and, per §2, retried up to four
	// times. A declaration already at or past this event's status has had it
	// applied, so re-running would complete an already-finished step and move
	// the trader's workflow on a second time. Nothing is modified; the
	// callback is simply acknowledged.
	if decl.Status.hasReached(targetStatus) {
		slog.InfoContext(ctx, "event already applied, acknowledging without action",
			"cusdec_ref", ref, "event", req.Event, "status", decl.Status)
		return ErrDuplicateEvent
	}

	return s.completeEventTaskAndMetadata(ctx, decl, taskTemplateID, targetStatus, payload, req)
}

func (s *webhookService) completeEventTaskAndMetadata(
	ctx context.Context,
	decl *CusdecDeclaration,
	taskTemplateID string,
	targetStatus CusdecStatus,
	payload map[string]any,
	req CusdecEventRequest,
) error {
	// Looked for across the whole case, not only the workflow the declaration
	// was dispatched from: the customs flow waits on export release in a branch
	// of the parallel split after the CDNs, a child workflow of its own.
	task, ok, err := parkedtask.FindInCase(ctx, s.tasks, decl.CaseID, taskTemplateID)
	if err != nil {
		// The status is left as it was: recording it would make SLC Edge's
		// retry a duplicate, and the parked step would never be completed.
		return fmt.Errorf("locate the %s task in case %s: %w", req.Event, decl.CaseID, err)
	}
	if !ok {
		if decl.Status == targetStatus {
			slog.InfoContext(ctx, "event task not found but CusDec status already updated", "edge_id", decl.EdgeID, "event", req.Event)
			return nil
		}
		slog.WarnContext(ctx, "event task not found, recovering status update", "edge_id", decl.EdgeID, "event", req.Event, "case_id", decl.CaseID)
		decl.Status = targetStatus
		if updateErr := s.repo.Update(ctx, decl); updateErr != nil {
			return fmt.Errorf("failed to update status during recovery: %w", updateErr)
		}
		return nil
	}

	if err := s.taskManager.CompleteTaskStep(ctx, task.TaskID, task.StepID, payload); err != nil {
		slog.ErrorContext(ctx, "failed to complete event task step", "task_id", task.TaskID, "error", err)
		return fmt.Errorf("failed to complete task step for task %s: %w", task.TaskID, err)
	}

	decl.Status = targetStatus
	if err := s.repo.Update(ctx, decl); err != nil {
		return fmt.Errorf("failed to update CusDec declaration status to %s: %w", targetStatus, err)
	}

	slog.InfoContext(ctx, "successfully processed CusDec event notification", "edge_id", decl.EdgeID, "status", targetStatus)
	return nil
}
