package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"

	"github.com/OpenNSW/core/pagination"
	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/caseparty"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
	nswtasks "github.com/OpenNSW/nsw-srilanka/internal/tasks"
)

// TaskStore is the narrow interface needed from taskv2 package to load task records.
type TaskStore interface {
	// GetTasks lists the tasks filter matches. The consignment detail and the engine status
	// pass only a CaseID, so every task of the case is listed whatever its read roles: the
	// detail authorizes its caller on the consignment, and the engine status is an admin view.
	GetTasks(ctx context.Context, filter store.TaskFilter) []store.TaskRecord
	GetTaskByWorkflowID(ctx context.Context, workflowID string) (store.TaskRecord, bool)
}

// CaseFlows creates a case and starts and lists its flows. *caseflow.Service satisfies it.
type CaseFlows interface {
	EnsureCase(ctx context.Context, caseID string) error
	StartFlow(ctx context.Context, req caseflow.StartRequest) (*caseflow.Flow, error)
	Flows(ctx context.Context, caseID string) ([]caseflow.Flow, error)
}

// Service handles consignment-related operations. A consignment is a case (see
// internal/caseflow): its flows are started through flows, which also keeps the case's
// state in step with them.
type Service struct {
	db        *gorm.DB
	artifacts *artifact.Set
	flows     CaseFlows
	// wm is the parent workflow manager, read for engine status and admin resolution.
	wm workflow.Manager
	// taskWm is the workflow.Manager for the per-task ("micro") workflow runner — a separate
	// manager/task queue from wm.
	taskWm         workflow.Manager
	chaService     cha.Service
	companyService company.Service
	userService    user.Service
	taskStore      TaskStore
	// tasks closes a task from outside its workflow for ForceParkTaskNode.
	tasks TaskCanceller
	roles map[string]string // logical name ("trader"/"cha") -> IdP token role
	// userFlows are the flows a trader or CHA may start on a consignment (flows.users).
	userFlows caseflow.UserFlows
}

// NewService creates a new instance of Service. roles is the global catalog's Roles
// map; it must define "trader" and "cha" (validated via validateRoles, the same
// function NewRouter uses) so GetConsignmentByID can tie a caller's matched company
// slot to the JWT role required for that slot. userFlows are the flows a trader or CHA may
// start on a consignment (flows.users).
func NewService(
	db *gorm.DB,
	artifacts *artifact.Set,
	chaService cha.Service,
	companyService company.Service,
	userService user.Service,
	taskStore TaskStore,
	roles map[string]string,
	userFlows caseflow.UserFlows,
) (*Service, error) {
	if err := validateRoles(roles); err != nil {
		return nil, err
	}
	return &Service{
		userFlows:      userFlows,
		db:             db,
		artifacts:      artifacts,
		chaService:     chaService,
		companyService: companyService,
		userService:    userService,
		taskStore:      taskStore,
		roles:          roles,
	}, nil
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

// RegisterTaskWorkflowManager registers the workflow manager for the per-task ("micro")
// workflow runner.
func (s *Service) RegisterTaskWorkflowManager(wm workflow.Manager) error {
	if s.taskWm != nil {
		return fmt.Errorf("task workflow manager already registered for ConsignmentService")
	}
	if wm == nil {
		return fmt.Errorf("task workflow manager cannot be nil")
	}
	s.taskWm = wm
	return nil
}

// RegisterCaseFlows registers what starts and lists the consignment's flows.
func (s *Service) RegisterCaseFlows(flows CaseFlows) error {
	if s.flows != nil {
		return fmt.Errorf("case flows already registered for ConsignmentService")
	}
	if flows == nil {
		return fmt.Errorf("case flows cannot be nil")
	}
	s.flows = flows
	return nil
}

// CreateAndStartConsignment creates an export consignment, with its cases row, and starts
// its export flow.
func (s *Service) CreateAndStartConsignment(ctx context.Context, traderID string, templateID string) (*DetailDTO, error) {
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

	// Checked before anything is written, so an unknown template creates no consignment.
	// caseflow starts the flow on the same current version.
	if _, err := workflowdef.Load(ctx, s.artifacts.CurrentRegistry(), templateID); err != nil {
		return nil, fmt.Errorf("failed to get workflow template %q: %w", templateID, err)
	}
	if s.flows == nil {
		return nil, fmt.Errorf("no case flows registered for ConsignmentService")
	}

	consignment := &Consignment{
		ID:              uuid.NewString(),
		Flow:            FlowExport,
		TraderID:        traderID,
		TraderCompanyID: traderCompany.ID,
	}
	// The case comes first: the consignment references it. caseflow creates it in its own
	// statement, so a failed consignment insert leaves an empty case behind, which is
	// harmless (a consignment is only ever listed through its consignments row).
	// TODO(#653): once consignments move off GORM to database/sql, create the case and the
	// consignment in one transaction (EnsureCase taking the *sql.Tx), so a failed insert
	// leaves no empty case.
	if err := s.flows.EnsureCase(ctx, consignment.ID); err != nil {
		return nil, fmt.Errorf("failed to create case: %w", err)
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return createConsignment(ctx, tx, consignment, traderCompany.OUHandle)
	}); err != nil {
		return nil, err
	}

	// The start runs after the commit, so the flow is recorded against a case that exists.
	if _, err := s.flows.StartFlow(ctx, caseflow.StartRequest{
		CaseID:     consignment.ID,
		TemplateID: templateID,
		Vars:       initialVars,
		Case:       &caseflow.CaseInfo{Attributes: caseAttributes(traderCompany)},
	}); err != nil {
		return nil, fmt.Errorf("failed to start the export flow: %w", err)
	}

	consignment, err = s.fetchConsignmentRecord(ctx, consignment.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload consignment: %w", err)
	}

	// Its creator is its trader.
	responseDTO, err := s.buildConsignmentDetailDTO(ctx, consignment, s.userFlows.Startable([]string{partyRoleTrader}))
	if err != nil {
		return nil, err
	}
	return responseDTO, nil
}

// GetConsignmentByID retrieves a consignment by its ID from the database, scoped to the
// caller's company AND the JWT role tied to whichever slot that company matched: a caller
// whose company is the trader company must also hold the "trader" role, and a caller whose
// company is the CHA company must also hold the "cha" role. Matching a company alone is not
// enough (#272) — e.g. a Trader-only user whose company happens to be the CHA company on some
// other consignment must not be granted access through that slot. If a company is both the
// trader and CHA company on the same row, holding either role is sufficient. callerRoles may
// be nil; slices.Contains on a nil slice reports false, so a caller with no roles is denied,
// never panics. Otherwise ErrAccessDenied is returned before the workflow engine or task store
// is touched, so an unauthorized caller triggers no work beyond the single row read.
func (s *Service) GetConsignmentByID(ctx context.Context, consignmentID, callerCompanyID string, callerRoles []string) (*DetailDTO, error) {
	consignment, err := s.fetchConsignmentRecord(ctx, consignmentID)
	if err != nil {
		return nil, err
	}
	owned, err := s.authorize(consignment, callerCompanyID, callerRoles)
	if err != nil {
		return nil, err
	}
	return s.buildConsignmentResponse(ctx, consignment, s.userFlows.Startable(owned))
}

// authorize is GetConsignmentByID's access check (see there): the caller's company must be
// the consignment's trader company with the "trader" role, or its CHA company with the "cha"
// role. It returns the logical roles the caller owns the consignment in, or ErrAccessDenied
// when there are none.
func (s *Service) authorize(consignment *Consignment, callerCompanyID string, callerRoles []string) ([]string, error) {
	// The empty-callerCompanyID guard documents the invariant the check relies on (a
	// resolved company always has a non-null id) and ensures an empty caller id can never
	// match an unassigned (empty) CHA company.
	if callerCompanyID == "" {
		return nil, ErrAccessDenied
	}
	chaCompanyID := ""
	if consignment.CHACompanyID != nil {
		chaCompanyID = *consignment.CHACompanyID
	}
	var owned []string
	if callerCompanyID == consignment.TraderCompanyID && slices.Contains(callerRoles, s.roles[partyRoleTrader]) {
		owned = append(owned, partyRoleTrader)
	}
	if callerCompanyID == chaCompanyID && slices.Contains(callerRoles, s.roles[partyRoleCHA]) {
		owned = append(owned, partyRoleCHA)
	}
	if len(owned) == 0 {
		return nil, ErrAccessDenied
	}
	return owned, nil
}

// StartConsignmentFlow starts one of the configured flows (flows.users) on a consignment for
// a trader or CHA caller, under the same access check as GetConsignmentByID, and only a flow
// listed for a role the caller owns the consignment in. The caller only
// picks the flow: its variables are set here, from the consignment (consignmentId and the
// trader company, as the export flow gets), never from the request, so a caller cannot pre-set
// state the flow trusts. Anything the caller supplies is collected by the flow's own tasks. A
// consignment that has FINISHED still takes new flows; starting one sets it back to
// IN_PROGRESS.
func (s *Service) StartConsignmentFlow(
	ctx context.Context, consignmentID, templateID string, callerCompanyID string, callerRoles []string,
) (*FlowDTO, error) {
	if s.flows == nil {
		return nil, fmt.Errorf("no case flows registered for ConsignmentService")
	}
	consignment, err := s.fetchConsignmentRecord(ctx, consignmentID)
	if err != nil {
		return nil, err
	}
	owned, err := s.authorize(consignment, callerCompanyID, callerRoles)
	if err != nil {
		return nil, err
	}
	if _, ok := s.userFlows.Find(templateID, owned); !ok {
		return nil, fmt.Errorf("%w: %q", ErrFlowNotAllowed, templateID)
	}

	traderCompany, err := s.companyService.GetCompanyByID(ctx, consignment.TraderCompanyID)
	if err != nil {
		return nil, fmt.Errorf("trader company lookup failed: %w", err)
	}
	traderCompanyVars, err := companyRecordToMap(traderCompany)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trader company: %w", err)
	}
	vars := map[string]any{"consignmentId": consignmentID, "traderCompany": traderCompanyVars}
	f, err := s.flows.StartFlow(ctx, caseflow.StartRequest{CaseID: consignmentID, TemplateID: templateID, Vars: vars})
	if err != nil {
		return nil, fmt.Errorf("failed to start flow %q on consignment %s: %w", templateID, consignmentID, err)
	}
	dto := s.flowDTO(ctx, *f)
	return &dto, nil
}

// GetConsignmentByIDForAdmin returns the same full consignment detail as GetConsignmentByID, plus
// the consignment's flows, but performs no trader/CHA ownership check — callers must already be
// gated to ConsignmentAdminRead by the router (see HandleAdminGetConsignmentByID).
func (s *Service) GetConsignmentByIDForAdmin(ctx context.Context, consignmentID string) (*AdminDetailDTO, error) {
	consignment, err := s.fetchConsignmentRecord(ctx, consignmentID)
	if err != nil {
		return nil, err
	}
	// The admin view starts no flows, but lists every one a trader or CHA could.
	detail, err := s.buildConsignmentResponse(ctx, consignment, s.userFlows.Startable([]string{partyRoleTrader, partyRoleCHA}))
	if err != nil {
		return nil, err
	}
	flows, err := s.flowDTOs(ctx, consignmentID)
	if err != nil {
		return nil, err
	}
	return &AdminDetailDTO{DetailDTO: *detail, Flows: flows}, nil
}

// flowDTOs lists the consignment's flows, oldest first.
func (s *Service) flowDTOs(ctx context.Context, consignmentID string) ([]FlowDTO, error) {
	if s.flows == nil {
		return nil, fmt.Errorf("no case flows registered for ConsignmentService")
	}
	flows, err := s.flows.Flows(ctx, consignmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list flows of consignment %s: %w", consignmentID, err)
	}
	dtos := make([]FlowDTO, 0, len(flows))
	for _, f := range flows {
		dtos = append(dtos, s.flowDTO(ctx, f))
	}
	return dtos, nil
}

// flowDTO presents f, named by its workflow definition's name in the manifest version
// the flow runs on (its template ID when the definition has none or cannot be loaded).
func (s *Service) flowDTO(ctx context.Context, f caseflow.Flow) FlowDTO {
	name := f.TemplateID
	if reg, err := s.artifacts.Registry(ctx, f.ManifestVersion); err == nil {
		if def, err := workflowdef.Load(ctx, reg, f.TemplateID); err == nil && def.Name != "" {
			name = def.Name
		}
	}
	return FlowDTO{
		WorkflowID:   f.WorkflowID,
		TemplateID:   f.TemplateID,
		TemplateName: name,
		Status:       string(f.Status),
		CreatedAt:    f.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    f.UpdatedAt.Format(time.RFC3339),
	}
}

// Roles the consignment's companies hold on its case (case_parties).
const (
	partyRoleTrader = "trader"
	partyRoleCHA    = "cha"
)

// consignmentColumns selects a Consignment from withCase: Name, State and the trader user
// from its cases row, and the trader and CHA companies from the companies of its parties.
const consignmentColumns = "consignments.id, consignments.flow, consignments.created_at, consignments.updated_at, " +
	"cases.name, cases.state, cases.created_by AS trader_id, " +
	"trader_company.id AS trader_company_id, cha_company.id AS cha_company_id"

// withCase joins consignments to their cases row, which holds Name, State and the trader
// user, to the case's trader and CHA parties, and to the company each party's OU is.
// Rows are read with Select(consignmentColumns); columns several tables have (id,
// timestamps) are qualified in conditions.
func (s *Service) withCase(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&Consignment{}).
		Joins("JOIN cases ON cases.id = consignments.id").
		Joins("LEFT JOIN case_parties trader_party ON trader_party.case_id = consignments.id AND trader_party.role = ? AND trader_party.party_type = ?",
			partyRoleTrader, caseparty.TypeOU).
		Joins("LEFT JOIN company_records trader_company ON trader_company.ou_handle = trader_party.party_id").
		Joins("LEFT JOIN case_parties cha_party ON cha_party.case_id = consignments.id AND cha_party.role = ? AND cha_party.party_type = ?",
			partyRoleCHA, caseparty.TypeOU).
		Joins("LEFT JOIN company_records cha_company ON cha_company.ou_handle = cha_party.party_id")
}

// createConsignment inserts c, records its trader company's OU (traderOUHandle) as the
// case's "trader" party and its trader user as the case's creator, on tx so they commit
// together. The case row must already exist.
func createConsignment(ctx context.Context, tx *gorm.DB, c *Consignment, traderOUHandle string) error {
	if err := tx.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("failed to create consignment: %w", err)
	}
	if err := caseparty.Assign(ctx, tx, caseparty.Party{
		CaseID:     c.ID,
		Role:       partyRoleTrader,
		Type:       caseparty.TypeOU,
		ID:         traderOUHandle,
		AssignedBy: &c.TraderID,
	}); err != nil {
		return fmt.Errorf("failed to record the consignment's trader: %w", err)
	}
	if err := tx.WithContext(ctx).Exec(`UPDATE cases SET created_by = ? WHERE id = ?`, c.TraderID, c.ID).Error; err != nil {
		return fmt.Errorf("failed to record the consignment's creator: %w", err)
	}
	return nil
}

func (s *Service) fetchConsignmentRecord(ctx context.Context, consignmentID string) (*Consignment, error) {
	var consignment Consignment
	result := s.withCase(ctx).Select(consignmentColumns).First(&consignment, "consignments.id = ?", consignmentID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrConsignmentNotFound
		}
		return nil, fmt.Errorf("failed to retrieve consignment with ID %s: %w", consignmentID, result.Error)
	}
	return &consignment, nil
}

// buildConsignmentResponse builds consignment's detail. startable are the flows the reader
// may start on it (DetailDTO.StartableFlows).
func (s *Service) buildConsignmentResponse(ctx context.Context, consignment *Consignment, startable []caseflow.UserFlow) (*DetailDTO, error) {
	responseDTO, err := s.buildConsignmentDetailDTO(ctx, consignment, startable)
	if err != nil {
		return nil, fmt.Errorf("failed to build consignment response DTO: %w", err)
	}

	return responseDTO, nil
}

// GetAgencySummary returns the allowlisted projection for an Agency caller.
// It does not enforce trader/CHA ownership — the route is gated by
// consignment:read and an unguessable consignment UUID.
func (s *Service) GetAgencySummary(ctx context.Context, consignmentID string) (*AgencyDTO, error) {
	consignment, err := s.fetchConsignmentRecord(ctx, consignmentID)
	if err != nil {
		return nil, err
	}

	traderCompany, err := s.companyService.GetCompanyByID(ctx, consignment.TraderCompanyID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve trader company with ID %s: %w", consignment.TraderCompanyID, err)
	}

	return &AgencyDTO{
		ConsignmentID:     consignment.ID,
		TraderCompanyName: traderCompany.Name,
	}, nil
}

// ListConsignments returns consignments scoped to a company: those it is the trader on
// (TraderCompanyID), those it is the CHA on (CHACompanyID), or either when both are set. At
// least one of the two must be set.
func (s *Service) ListConsignments(ctx context.Context, filter Filter) (*ListResult, error) {
	var baseQuery *gorm.DB
	switch {
	case filter.TraderCompanyID != nil && filter.CHACompanyID != nil:
		baseQuery = s.withCase(ctx).Where("(trader_company.id = ? OR cha_company.id = ?)", *filter.TraderCompanyID, *filter.CHACompanyID)
	case filter.TraderCompanyID != nil:
		baseQuery = s.withCase(ctx).Where("trader_company.id = ?", *filter.TraderCompanyID)
	case filter.CHACompanyID != nil:
		baseQuery = s.withCase(ctx).Where("cha_company.id = ?", *filter.CHACompanyID)
	default:
		return nil, fmt.Errorf("at least one of TraderCompanyID or CHACompanyID must be set in filter")
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
			q = q.Where("cases.state = ?", *filter.State)
		}
		if filter.Flow != nil {
			q = q.Where("flow = ?", *filter.Flow)
		}
		if filter.Query != nil && *filter.Query != "" {
			escaped := strings.ReplaceAll(*filter.Query, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "%", "\\%")
			escaped = strings.ReplaceAll(escaped, "_", "\\_")
			queryVal := "%" + strings.ToLower(escaped) + "%"
			q = q.Where("LOWER(consignments.id) LIKE ? ESCAPE '\\' OR LOWER(cases.name) LIKE ? ESCAPE '\\'", queryVal, queryVal)
		}
		return q
	}

	var consignments []Consignment
	if err := filteredQuery().
		WithContext(ctx).
		Select(consignmentColumns).
		Offset(finalOffset).
		Limit(finalLimit).
		Order("consignments.created_at DESC").
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

		chaCompanyID := ""
		if c.CHACompanyID != nil {
			chaCompanyID = *c.CHACompanyID
		}

		name := ""
		if c.Name != nil {
			name = *c.Name
		}

		consignmentDTOs = append(consignmentDTOs, SummaryDTO{
			ID:              c.ID,
			Name:            name,
			Flow:            c.Flow,
			State:           c.State,
			TraderID:        c.TraderID,
			TraderCompanyID: c.TraderCompanyID,
			ChaCompanyID:    chaCompanyID,
			CallerRoles:     callerRolesOn(c.TraderCompanyID, chaCompanyID, filter),
			CreatedAt:       c.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       c.UpdatedAt.Format(time.RFC3339),
		})
	}

	result := pagination.NewPageResult(consignmentDTOs, totalCount, finalOffset, finalLimit)
	return &result, nil
}

// callerRolesOn reports the logical roles the caller acts in on a consignment with
// these trader and CHA companies. The handler sets a company filter only for a role
// the caller holds, so a filter matching the consignment's slot means the caller
// acts in that role there.
func callerRolesOn(traderCompanyID, chaCompanyID string, filter Filter) []string {
	roles := []string{}
	if filter.TraderCompanyID != nil && traderCompanyID == *filter.TraderCompanyID {
		roles = append(roles, "trader")
	}
	if filter.CHACompanyID != nil && chaCompanyID == *filter.CHACompanyID {
		roles = append(roles, "cha")
	}
	return roles
}

// buildConsignmentDetailDTO builds a DetailDTO from a Consignment. startable are the flows
// the reader may start on it.
func (s *Service) buildConsignmentDetailDTO(
	ctx context.Context,
	consignment *Consignment,
	startable []caseflow.UserFlow,
) (*DetailDTO, error) {
	nodeResponseDTOs, err := s.buildNodeDTOsFromTaskRecords(ctx, consignment.ID)
	if err != nil {
		return nil, err
	}

	chaCompanyID := ""
	if consignment.CHACompanyID != nil {
		chaCompanyID = *consignment.CHACompanyID
	}

	name := ""
	if consignment.Name != nil {
		name = *consignment.Name
	}

	return &DetailDTO{
		ID:              consignment.ID,
		Name:            name,
		Flow:            consignment.Flow,
		State:           consignment.State,
		TraderID:        consignment.TraderID,
		TraderCompanyID: consignment.TraderCompanyID,
		ChaCompanyID:    chaCompanyID,
		CreatedAt:       consignment.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       consignment.UpdatedAt.Format(time.RFC3339),
		WorkflowNodes:   nodeResponseDTOs,
		StartableFlows:  flowOptions(startable),
	}, nil
}

// buildNodeDTOsFromTaskRecords queries the consignment's tasks via the TaskStore by case ID and converts each
// non-SYSTEM record into a WorkflowNodeResponseDTO for the consignment detail response.
func (s *Service) buildNodeDTOsFromTaskRecords(ctx context.Context, consignmentID string) ([]WorkflowNodeResponseDTO, error) {
	if s.taskStore == nil {
		return nil, fmt.Errorf("task store not initialized")
	}
	tasks := s.taskStore.GetTasks(ctx, store.TaskFilter{CaseID: consignmentID})

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
		case "QUEUED_EXTERNALLY":
			nodeState = WorkflowNodeStateQueuedExternally
		case store.StateCancelled:
			nodeState = WorkflowNodeStateCancelled
		case store.StateOnHold:
			nodeState = WorkflowNodeStateOnHold
		default:
			// PENDING_USER and any other active orchestrator state: the trader
			// still has to act (including after an officer send-back).
			nodeState = WorkflowNodeStateInProgress
		}
		dtos = append(dtos, WorkflowNodeResponseDTO{
			ID:        t.TaskID,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
			UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
			WorkflowNodeTemplate: WorkflowNodeTemplateResponseDTO{
				Name: nswtasks.Title(ctx, nswtasks.RenderConfigs(s.artifacts), t),
				Type: t.TaskType,
			},
			State: nodeState,
		})
	}
	return dtos, nil
}

// SharedCaseAttributes are the keys of a case's attributes that EXTERNAL_REVIEW sends to
// the agencies it starts flows on (see caseflow.SharedAttributes). A key not listed here
// never leaves TNSW, whatever writes it to cases.attributes.
var SharedCaseAttributes = []string{"traderCompany"}

// caseAttributes are the attributes of the consignment's case (cases.attributes). Their
// keys are in SharedCaseAttributes, so only values every agency may see go in: the trader
// company's name, but not its id or data, nor the consignment's own name.
func caseAttributes(trader *company.Record) map[string]any {
	return map[string]any{"traderCompany": map[string]any{"name": trader.Name}}
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
