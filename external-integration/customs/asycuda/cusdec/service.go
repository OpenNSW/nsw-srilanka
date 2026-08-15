package cusdec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WebhookService defines the use-case layer for processing asynchronous
// ASYCUDA callbacks related to Customs Declarations.
type WebhookService interface {
	ProcessIntegrationResult(ctx context.Context, req CusdecIntegrationResultRequest) error
	ProcessEvent(ctx context.Context, req CusdecEventRequest) error
}

// TaskCompleter defines the task completion interface needed from the workflow
// manager to advance suspended tasks.
type TaskCompleter interface {
	CompleteTaskStep(ctx context.Context, taskID string, payload map[string]any) error
}

type webhookService struct {
	repo        DeclarationRepository
	db          *gorm.DB
	taskManager TaskCompleter
}

// NewWebhookService creates a new WebhookService.
func NewWebhookService(repo DeclarationRepository, db *gorm.DB, taskManager TaskCompleter) WebhookService {
	return &webhookService{
		repo:        repo,
		db:          db,
		taskManager: taskManager,
	}
}

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
	existing, err := s.repo.GetByEdgeID(ctx, req.EdgeID)
	if err != nil {
		return fmt.Errorf("failed to retrieve CusDec declaration by edgeId %s: %w", req.EdgeID, err)
	}
	if existing != nil && (existing.Status == CusdecStatusIntegrated || existing.Status == CusdecStatusFailed) {
		slog.InfoContext(ctx, "integration result already processed, acknowledging without action",
			"edge_id", req.EdgeID, "status", existing.Status)
		return ErrDuplicateIntegrationResult
	}

	decl, originalStatus, err := s.updateCusdecDeclaration(ctx, req)
	if err != nil {
		return err
	}

	if err := s.completeReviewTask(ctx, decl, originalStatus, req); err != nil {
		return err
	}

	slog.InfoContext(ctx, "successfully completed external review task step and advanced workflow",
		"edge_id", req.EdgeID,
		"integrated", req.Integrated,
	)
	return nil
}

func (s *webhookService) updateCusdecDeclaration(ctx context.Context, req CusdecIntegrationResultRequest) (*CusdecDeclaration, CusdecStatus, error) {
	decl, err := s.repo.GetByEdgeID(ctx, req.EdgeID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to retrieve CusDec declaration by edgeId %s: %w", req.EdgeID, err)
	}

	originalStatus := CusdecStatus("")
	isNew := false
	if decl == nil {
		isNew = true
		decl = &CusdecDeclaration{
			ID:     uuid.NewString(),
			EdgeID: req.EdgeID,
		}
	} else {
		originalStatus = decl.Status
	}

	if decl.Status != CusdecStatusIntegrated && decl.Status != CusdecStatusFailed {
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

		if isNew {
			if err := s.repo.Create(ctx, decl); err != nil {
				return nil, "", fmt.Errorf("failed to create CusDec declaration for edgeId %s: %w", decl.EdgeID, err)
			}
		} else {
			if err := s.repo.Update(ctx, decl); err != nil {
				return nil, "", fmt.Errorf("failed to update CusDec declaration for edgeId %s: %w", decl.EdgeID, err)
			}
		}
	}
	return decl, originalStatus, nil
}

func (s *webhookService) completeReviewTask(ctx context.Context, decl *CusdecDeclaration, originalStatus CusdecStatus, req CusdecIntegrationResultRequest) error {
	var record struct {
		ParentWorkflowID string `gorm:"column:parent_workflow_id"`
	}
	err := s.db.WithContext(ctx).
		Table("task_records_v2").
		Where("data->'cig'->>'edgeId' = ? OR data->'cig'->>'edge_id' = ?", req.EdgeID, req.EdgeID).
		Select("parent_workflow_id").
		First(&record).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if originalStatus != CusdecStatusSubmitted {
				slog.InfoContext(ctx, "workflow record not found but CusDec declaration was already processed, ignoring duplicate callback", "edge_id", req.EdgeID, "status", originalStatus)
				return nil
			}
			return fmt.Errorf("edgeId %s: %w", req.EdgeID, ErrWorkflowNotFoundByEdgeID)
		}
		slog.ErrorContext(ctx, "failed to locate task workflow by edgeId", "edge_id", req.EdgeID, "error", err)
		return fmt.Errorf("failed to locate task workflow by edgeId %s: %w", req.EdgeID, err)
	}

	var task struct {
		TaskID string `gorm:"column:task_id"`
	}
	err = s.db.WithContext(ctx).
		Table("task_records_v2").
		Where("parent_workflow_id = ? AND active_task_template_id = ? AND state = ?",
			record.ParentWorkflowID, "customs-cusdec--external-review", "QUEUED_EXTERNALLY").
		Select("task_id").
		First(&task).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if originalStatus != CusdecStatusSubmitted {
				slog.InfoContext(ctx, "external review task not found but CusDec declaration was already processed, ignoring duplicate callback", "edge_id", req.EdgeID, "status", originalStatus)
				return nil
			}
		}
		slog.ErrorContext(ctx, "failed to locate suspended external review task", "workflow_id", record.ParentWorkflowID, "error", err)
		return fmt.Errorf("failed to locate suspended external review task for workflow %s: %w", record.ParentWorkflowID, err)
	}

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
			// §6.2 returns the assessed taxes; their total is what the trader
			// is asked to settle on the payment step that follows.
			"amount_to_pay": totalTaxes(req.Payload.Taxes),
		}
	} else {
		payload = map[string]any{
			"__command":        "submit",
			"review_outcome":   "needs_more_info",
			"rejection_reason": describeErrors(req.Errors),
		}
	}

	if err := s.taskManager.CompleteTaskStep(ctx, task.TaskID, payload); err != nil {
		slog.ErrorContext(ctx, "failed to complete external review task step", "task_id", task.TaskID, "error", err)
		return fmt.Errorf("failed to complete task step for task %s: %w", task.TaskID, err)
	}

	return nil
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
	var record struct {
		ParentWorkflowID string `gorm:"column:parent_workflow_id"`
	}
	err := s.db.WithContext(ctx).
		Table("task_records_v2").
		Where("data->'cig'->>'edgeId' = ? OR data->'cig'->>'edge_id' = ?", decl.EdgeID, decl.EdgeID).
		Select("parent_workflow_id").
		First(&record).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if decl.Status == targetStatus {
				slog.InfoContext(ctx, "workflow record not found but CusDec status already updated", "edge_id", decl.EdgeID, "event", req.Event)
				return nil
			}
			slog.WarnContext(ctx, "workflow record not found, recovering status update", "edge_id", decl.EdgeID, "event", req.Event)
			decl.Status = targetStatus
			if updateErr := s.repo.Update(ctx, decl); updateErr != nil {
				return fmt.Errorf("failed to update status during recovery: %w", updateErr)
			}
			return nil
		}
		return fmt.Errorf("failed to locate workflow record: %w", err)
	}

	var task struct {
		TaskID string `gorm:"column:task_id"`
	}
	err = s.db.WithContext(ctx).
		Table("task_records_v2").
		Where("parent_workflow_id = ? AND active_task_template_id = ? AND state = ?",
			record.ParentWorkflowID, taskTemplateID, "QUEUED_EXTERNALLY").
		Select("task_id").
		First(&task).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if decl.Status == targetStatus {
				slog.InfoContext(ctx, "event task not found but CusDec status already updated", "edge_id", decl.EdgeID, "event", req.Event)
				return nil
			}
			slog.WarnContext(ctx, "event task not found, recovering status update", "edge_id", decl.EdgeID, "event", req.Event)
			decl.Status = targetStatus
			if updateErr := s.repo.Update(ctx, decl); updateErr != nil {
				return fmt.Errorf("failed to update status during recovery: %w", updateErr)
			}
			return nil
		}
		return fmt.Errorf("failed to locate event task %s: %w", taskTemplateID, err)
	}

	if err := s.taskManager.CompleteTaskStep(ctx, task.TaskID, payload); err != nil {
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
