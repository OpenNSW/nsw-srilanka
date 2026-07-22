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
			if originalStatus == CusdecStatusIntegrated || originalStatus == CusdecStatusFailed {
				slog.InfoContext(ctx, "workflow record not found but CusDec declaration was already processed, ignoring duplicate callback", "edge_id", req.EdgeID)
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
			if originalStatus == CusdecStatusIntegrated || originalStatus == CusdecStatusFailed {
				slog.InfoContext(ctx, "external review task not found but CusDec declaration was already processed, ignoring duplicate callback", "edge_id", req.EdgeID)
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
			"amount_to_pay":  0,
		}
	} else {
		payload = map[string]any{
			"__command":        "submit",
			"review_outcome":   "needs_more_info",
			"rejection_reason": string(req.Errors),
		}
	}

	if err := s.taskManager.CompleteTaskStep(ctx, task.TaskID, payload); err != nil {
		slog.ErrorContext(ctx, "failed to complete external review task step", "task_id", task.TaskID, "error", err)
		return fmt.Errorf("failed to complete task step for task %s: %w", task.TaskID, err)
	}

	return nil
}
