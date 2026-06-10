package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifactadapter/workflowdef"
	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"

	"github.com/OpenNSW/core/pagination"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

// TaskStore is the narrow interface needed from taskv2 package to load task records.
type TaskStore interface {
	GetAllTasks(ctx context.Context, parentWorkflowID string) []store.TaskRecord
}

// Service handles consignment-related operations.
// It coordinates between workflow templates, nodes, and the workflow manager.
// It also implements WorkflowEventHandler for domain-specific lifecycle callbacks.
type Service struct {
	db               *gorm.DB
	artifactRegistry *artifact.Registry
	wm               workflow.Manager
	chaService       cha.Service
	companyService   company.Service
	userService      user.Service
	taskStore        TaskStore
}

// NewService creates a new instance of Service.
func NewService(
	db *gorm.DB,
	artifactRegistry *artifact.Registry,
	chaService cha.Service,
	companyService company.Service,
	userService user.Service,
	taskStore TaskStore,
) *Service {
	return &Service{
		db:               db,
		artifactRegistry: artifactRegistry,
		chaService:       chaService,
		companyService:   companyService,
		userService:      userService,
		taskStore:        taskStore,
	}
}

// RegisterWorkflowManager registers the workflow manager.
func (s *Service) RegisterWorkflowManager(wm workflow.Manager) error {
	if s.wm != nil {
		return fmt.Errorf("workflow manager already registered for ConsignmentService")
	}
	if wm == nil {
		return fmt.Errorf("workflow manager cannot be nil")
	}
	s.wm = wm
	return nil
}

// startWorkflow starts a workflow on the registered workflow manager.
func (s *Service) startWorkflow(ctx context.Context, workflowID string, def workflow.WorkflowDefinition, vars map[string]any) error {
	if s.wm == nil {
		return fmt.Errorf("no workflow manager registered for ConsignmentService")
	}
	return s.wm.StartWorkflow(ctx, workflowID, def, vars)
}

// getWorkflowStatus checks that a workflow is reachable on the registered workflow manager.
func (s *Service) getWorkflowStatus(ctx context.Context, workflowID string) error {
	if s.wm == nil {
		return fmt.Errorf("no workflow manager registered for ConsignmentService")
	}
	_, err := s.wm.GetStatus(ctx, workflowID)
	return err
}

// CompletionHandler is called by the workflow runtime when a workflow completes.
func (s *Service) CompletionHandler(workflowID string, finalContext map[string]any) error {
	return s.OnWorkflowStatusChanged(context.Background(), s.db, workflowID, WorkflowStatusCompleted)
}

// OnWorkflowStatusChanged handles workflow lifecycle state propagation to consignment domain state.
func (s *Service) OnWorkflowStatusChanged(ctx context.Context, tx *gorm.DB, workflowID string, toStatus WorkflowStatus) error {
	switch toStatus {
	case WorkflowStatusCompleted:
		return s.markConsignmentAsFinished(ctx, tx, workflowID)
	default:
		return nil
	}
}

// directStartExportWorkflowTemplateID is the top-level workflow started immediately by
// CreateAndStartConsignment. CHA selection happens inside this workflow's own
// tasks (trade_1_cha_selection) as workflow variables
// (trade.cha_id) rather than as an upfront trader/CHA handoff.
const directStartExportWorkflowTemplateID = "trade-export-v1"

// CreateAndStartConsignment creates an export consignment and starts its workflow directly,
// in one step — replacing the two-stage trader-creates-shell → CHA-claims handoff
// for flows whose entire CHA selection now happens inside the workflow itself.
func (s *Service) CreateAndStartConsignment(ctx context.Context, traderID string) (*DetailDTO, error) {
	traderUser, err := s.userService.GetUser(ctx, traderID)
	if err != nil {
		return nil, fmt.Errorf("trader user lookup failed: %w", err)
	}

	traderCompany, err := s.companyService.GetCompanyByOUHandle(ctx, traderUser.OUHandle)
	if err != nil {
		return nil, fmt.Errorf("trader company lookup failed: %w", err)
	}

	traderCompanyVars, err := companyRecordToMap(traderCompany)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trader company: %w", err)
	}
	initialVars := map[string]any{"traderCompany": traderCompanyVars}

	def, err := workflowdef.Load(ctx, s.artifactRegistry, directStartExportWorkflowTemplateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow template: %w", err)
	}

	consignment := &Consignment{
		ID:              uuid.NewString(),
		Flow:            FlowExport,
		TraderID:        traderID,
		TraderCompanyID: traderCompany.ID,
		State:           InProgress,
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	if err := tx.Create(consignment).Error; err != nil {
		return nil, fmt.Errorf("failed to create consignment: %w", err)
	}

	if err := s.startWorkflow(ctx, consignment.ID, def, initialVars); err != nil {
		return nil, fmt.Errorf("failed to register workflow: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	if err := s.db.WithContext(ctx).First(consignment, "id = ?", consignment.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload consignment: %w", err)
	}

	responseDTO, err := s.buildConsignmentDetailDTO(ctx, consignment)
	if err != nil {
		return nil, err
	}
	return responseDTO, nil
}

// GetConsignmentByID retrieves a consignment by its ID from the database.
func (s *Service) GetConsignmentByID(ctx context.Context, consignmentID string) (*DetailDTO, error) {
	var consignment Consignment
	result := s.db.WithContext(ctx).First(&consignment, "id = ?", consignmentID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrConsignmentNotFound
		}
		return nil, fmt.Errorf("failed to retrieve consignment with ID %s: %w", consignmentID, result.Error)
	}

	if err := s.getWorkflowStatus(ctx, consignment.ID); err != nil {
		slog.WarnContext(ctx, "workflow status check failed", "consignmentID", consignmentID, "error", err)
	}

	responseDTO, err := s.buildConsignmentDetailDTO(ctx, &consignment)
	if err != nil {
		return nil, fmt.Errorf("failed to build consignment response DTO: %w", err)
	}

	return responseDTO, nil
}

// ListConsignments returns consignments scoped to a company. For role=trader the caller passes
// TraderCompanyID; for role=cha the caller passes CHACompanyID. Exactly one of the two must be set.
func (s *Service) ListConsignments(ctx context.Context, filter Filter) (*ListResult, error) {
	var baseQuery *gorm.DB
	if filter.CHACompanyID != nil {
		baseQuery = s.db.WithContext(ctx).Model(&Consignment{}).Where("cha_company_id = ?", *filter.CHACompanyID)
	} else if filter.TraderCompanyID != nil {
		baseQuery = s.db.WithContext(ctx).Model(&Consignment{}).Where("trader_company_id = ?", *filter.TraderCompanyID)
	} else {
		return nil, fmt.Errorf("either TraderCompanyID or CHACompanyID must be set in filter")
	}
	return s.listConsignmentsWithBaseQuery(ctx, baseQuery, filter)
}

// listConsignmentsWithBaseQuery runs the shared list logic (filters, count, pagination, DTOs).
func (s *Service) listConsignmentsWithBaseQuery(ctx context.Context, baseQuery *gorm.DB, filter Filter) (*ListResult, error) {
	// Apply pagination with defaults and limits
	finalOffset, finalLimit := pagination.ResolvePaginationParams(filter.Offset, filter.Limit)

	filteredQuery := func() *gorm.DB {
		q := baseQuery
		if filter.State != nil {
			q = q.Where("state = ?", *filter.State)
		}
		if filter.Flow != nil {
			q = q.Where("flow = ?", *filter.Flow)
		}
		return q
	}

	var consignments []Consignment
	if err := filteredQuery().
		WithContext(ctx).
		Offset(finalOffset).
		Limit(finalLimit).
		Order("created_at DESC").
		Find(&consignments).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve consignments: %w", err)
	}

	var totalCount int64
	if len(consignments) < finalLimit && finalOffset == 0 {
		totalCount = int64(len(consignments))
	} else {
		if err := filteredQuery().Count(&totalCount).Error; err != nil {
			return nil, fmt.Errorf("failed to count filtered consignments: %w", err)
		}
	}

	if len(consignments) == 0 {
		result := pagination.NewPageResult([]SummaryDTO{}, totalCount, finalOffset, finalLimit)
		return &result, nil
	}

	// Build Summary DTOs for all consignments
	var consignmentDTOs []SummaryDTO
	for i := range consignments {
		c := consignments[i]

		chaID := ""
		if c.CHAID != nil {
			chaID = *c.CHAID
		}
		chaCompanyID := ""
		if c.CHACompanyID != nil {
			chaCompanyID = *c.CHACompanyID
		}

		consignmentDTOs = append(consignmentDTOs, SummaryDTO{
			ID:              c.ID,
			Flow:            c.Flow,
			State:           c.State,
			TraderID:        c.TraderID,
			TraderCompanyID: c.TraderCompanyID,
			ChaCompanyID:    chaCompanyID,
			ChaID:           chaID,
			CreatedAt:       c.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       c.UpdatedAt.Format(time.RFC3339),
		})
	}

	result := pagination.NewPageResult(consignmentDTOs, totalCount, finalOffset, finalLimit)
	return &result, nil
}

// markConsignmentAsFinished updates the consignment state to FINISHED.
func (s *Service) markConsignmentAsFinished(ctx context.Context, tx *gorm.DB, consignmentID string) error {
	var consignment Consignment
	if err := tx.WithContext(ctx).First(&consignment, "id = ?", consignmentID).Error; err != nil {
		return fmt.Errorf("failed to retrieve consignment %s: %w", consignmentID, err)
	}
	consignment.State = Finished
	if err := tx.WithContext(ctx).Save(&consignment).Error; err != nil {
		return fmt.Errorf("failed to update consignment %s state to FINISHED: %w", consignmentID, err)
	}
	return nil
}

// buildConsignmentDetailDTO builds a DetailDTO from a Consignment.
func (s *Service) buildConsignmentDetailDTO(
	ctx context.Context,
	consignment *Consignment,
) (*DetailDTO, error) {
	nodeResponseDTOs, err := s.buildNodeDTOsFromTaskRecords(ctx, consignment.ID)
	if err != nil {
		return nil, err
	}

	chaID := ""
	if consignment.CHAID != nil {
		chaID = *consignment.CHAID
	}
	chaCompanyID := ""
	if consignment.CHACompanyID != nil {
		chaCompanyID = *consignment.CHACompanyID
	}

	return &DetailDTO{
		ID:              consignment.ID,
		Flow:            consignment.Flow,
		State:           consignment.State,
		TraderID:        consignment.TraderID,
		TraderCompanyID: consignment.TraderCompanyID,
		ChaCompanyID:    chaCompanyID,
		ChaID:           chaID,
		CreatedAt:       consignment.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       consignment.UpdatedAt.Format(time.RFC3339),
		WorkflowNodes:   nodeResponseDTOs,
	}, nil
}

// buildNodeDTOsFromTaskRecords queries tasks via the TaskStore by root_workflow_id and converts each
// non-SYSTEM record into a WorkflowNodeResponseDTO for the consignment detail response.
func (s *Service) buildNodeDTOsFromTaskRecords(ctx context.Context, consignmentID string) ([]WorkflowNodeResponseDTO, error) {
	if s.taskStore == nil {
		return nil, fmt.Errorf("task store not initialized")
	}
	tasks := s.taskStore.GetAllTasks(ctx, consignmentID)

	dtos := make([]WorkflowNodeResponseDTO, 0, len(tasks))
	for _, t := range tasks {
		if t.TaskType == "SYSTEM" {
			continue
		}
		var nodeState WorkflowNodeState
		switch t.State {
		case "COMPLETED":
			nodeState = WorkflowNodeStateCompleted
		case "FAILED":
			nodeState = WorkflowNodeStateFailed
			// Note: the workflow manager doesn't currently have a FAILED state for tasks, but if it did,
			// we would want to reflect that here in the consignment detail response's workflow node states.
			// For now, any task that isn't marked as COMPLETED is effectively "in progress" from the API consumer's perspective.
			// This means that if a task fails, it will show as "in progress" in the UI until the workflow either retries or completes with an error.
			// When the workflow completes, the consignment will be marked as FINISHED regardless of whether individual tasks failed or succeeded, so the UI should primarily be checking the consignment state for a high-level view of whether the workflow is still active or fully completed.
		default:
			nodeState = WorkflowNodeStateInProgress
		}
		dtos = append(dtos, WorkflowNodeResponseDTO{
			ID:        t.TaskID,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
			UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
			WorkflowNodeTemplate: WorkflowNodeTemplateResponseDTO{
				Name: taskDisplayName(t.ActiveTaskTemplateID, t.RenderConfig),
				Type: t.TaskType,
			},
			State: nodeState,
		})
	}
	return dtos, nil
}

// taskDisplayName extracts the human-readable title from a task's render config workspace
// section, falling back to the active template ID when no title is present.
func taskDisplayName(templateID string, renderConfig json.RawMessage) string {
	if len(renderConfig) > 0 {
		var rc struct {
			Sections map[string]struct {
				Title string `json:"title"`
			} `json:"sections"`
		}
		if err := json.Unmarshal(renderConfig, &rc); err == nil {
			if ws, ok := rc.Sections["workspace"]; ok && ws.Title != "" {
				return ws.Title
			}
		}
	}
	return templateID
}

// companyRecordToMap converts a company.Record to a map[string]any via its JSON tags.
func companyRecordToMap(record *company.Record) (map[string]any, error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any)
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
