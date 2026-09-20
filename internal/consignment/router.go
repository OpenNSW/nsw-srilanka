package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/pagination"
	workflow "github.com/OpenNSW/core/workflow"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

const (
	errUnauthorized          = "unauthorized"
	errConsignmentIDRequired = "consignment ID is required"
	errInvalidRole           = "query param role must be trader or cha"
	errForbiddenRole         = "caller does not hold the requested role"
	errCompanyNotFound       = "company not found"
	errConsignmentNotFound   = "consignment not found"
	errWorkflowNotFound      = "workflow execution not found"
	errNodeIDRequired        = "node ID is required"
	errInvalidRequestBody    = "invalid request body"
	errInvalidAdminAction    = "action must be one of RETRY, OVERRIDE, SKIP, ABORT"
	errReasonRequired        = "reason is required"
)

// validAdminActions maps the wire-format action string onto the core/workflow constant, and
// doubles as the allowlist HandleResolveAdminIntervention validates against.
var validAdminActions = map[string]workflow.AdminResolutionAction{
	string(workflow.AdminActionRetry):    workflow.AdminActionRetry,
	string(workflow.AdminActionOverride): workflow.AdminActionOverride,
	string(workflow.AdminActionSkip):     workflow.AdminActionSkip,
	string(workflow.AdminActionAbort):    workflow.AdminActionAbort,
}

type Router struct {
	cs      *Service
	cha     cha.Service
	company company.Service
	audit   *nswaudit.Recorder
	roles   map[string]string // logical name ("trader"/"cha") -> IdP token role
}

// NewRouter builds the router. roles is the global catalog's Roles map; it must
// define "trader" and "cha" — HandleGetConsignments resolves a caller's ?role=
// query param through it.
func NewRouter(cs *Service, chaService cha.Service, companyService company.Service, recorder *nswaudit.Recorder, roles map[string]string) (*Router, error) {
	if err := validateRoles(roles); err != nil {
		return nil, err
	}
	return &Router{cs: cs, cha: chaService, company: companyService, audit: recorder, roles: roles}, nil
}

// validateRoles reports an error if roles (the global catalog's Roles map) omits
// "trader" or "cha", or maps either to an empty string — this package scopes
// queries by exactly those two names, so a missing one would silently deny every
// request in that role. Package-private; internal/consignment's
// Service.NewService also calls this (same package, no import needed).
func validateRoles(roles map[string]string) error {
	if err := catalog.RequireRoles(roles, "trader", "cha"); err != nil {
		return fmt.Errorf("consignment: %w", err)
	}
	return nil
}

// TODO: Move default workflow template ID to configuration.
// defaultExportWorkflowTemplateID is the top-level workflow started by default when
// creating a consignment.
const defaultExportWorkflowTemplateID = "trade-export-v1"

// HandleCreateConsignment handles POST /api/v1/consignments
// Creates an export consignment and starts its workflow directly — no CHA company or HS code
// is collected up front; the workflow's own tasks collect those later. Response: DetailDTO.
func (c *Router) HandleCreateConsignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}

	traderID := authCtx.User.ID
	consignment, err := c.cs.CreateAndStartConsignment(ctx, traderID, defaultExportWorkflowTemplateID)
	if err != nil {
		c.audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventConsignment,
			Action:     nswaudit.ActionCreate,
			TargetType: nswaudit.TargetConsignment,
			Failure:    true,
			Metadata: map[string]any{
				"error": err.Error(),
			},
		})
		httputil.InternalServerError(w, r, "failed to create and start consignment", err)
		return
	}
	if consignment == nil {
		c.audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventConsignment,
			Action:     nswaudit.ActionCreate,
			TargetType: nswaudit.TargetConsignment,
			Failure:    true,
			Metadata: map[string]any{
				"error": "consignment is nil after successful creation",
			},
		})
		httputil.InternalServerError(w, r, "consignment is nil after successful creation", nil)
		return
	}

	c.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventConsignment,
		Action:     nswaudit.ActionCreate,
		TargetType: nswaudit.TargetConsignment,
		TargetID:   consignment.ID,
		Failure:    false,
		Message:    consignment,
		Metadata: map[string]any{
			"flow":            consignment.Flow,
			"traderCompanyId": consignment.TraderCompanyID,
			"chaCompanyId":    consignment.ChaCompanyID,
		},
	})
	httputil.JSON(w, http.StatusCreated, consignment)
}

// buildConsignmentFilter parses optional query filters (state, flow, q) from the request.
func buildConsignmentFilter(r *http.Request, offset, limit *int) Filter {
	filter := Filter{Offset: offset, Limit: limit}
	if stateStr := r.URL.Query().Get("state"); stateStr != "" {
		state := State(stateStr)
		filter.State = &state
	}
	if flowStr := r.URL.Query().Get("flow"); flowStr != "" {
		flow := Flow(flowStr)
		filter.Flow = &flow
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		filter.Query = &q
	}
	return filter
}

// HandleGetConsignments handles GET /api/v1/consignments
// Query params: role=trader | role=cha (defaults to trader). The caller must hold
// the JWT role that maps to the requested role, or the request is forbidden.
func (c *Router) HandleGetConsignments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}

	role := r.URL.Query().Get("role")
	if role == "" {
		role = "trader"
	}
	offset, limit, err := pagination.ParsePaginationParams(r)
	if err != nil {
		slog.WarnContext(r.Context(), "invalid pagination parameters", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, "invalid pagination parameters")
		return
	}
	filter := buildConsignmentFilter(r, offset, limit)

	// Role-based identity resolution.
	if role != "trader" && role != "cha" {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRole)
		return
	}

	// The caller must actually hold the role they're asserting via the query
	// param — resolved through the global catalog, not hardcoded, so it stays in
	// step with the same "trader"/"cha" -> token-role mapping the task-authz
	// layer (internal/tasks/taskauthz) uses.
	requiredTokenRole, ok := c.roles[role]
	if !ok {
		httputil.InternalServerError(w, r, "role not configured in catalog", fmt.Errorf("catalog has no mapping for role %q", role))
		return
	}
	if !slices.Contains(authCtx.User.Roles, requiredTokenRole) {
		httputil.Error(w, r, http.StatusForbidden, errForbiddenRole)
		return
	}

	userCompany, err := c.company.GetCompanyByOUHandle(ctx, authCtx.User.OUHandle)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) || errors.Is(err, company.ErrInvalidCompanyID) {
			httputil.Error(w, r, http.StatusForbidden, errCompanyNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to resolve user company", err, "ouHandle", authCtx.User.OUHandle)
		return
	}

	switch role {
	case "cha":
		filter.CHACompanyID = &userCompany.ID
	case "trader":
		filter.TraderCompanyID = &userCompany.ID
	}
	consignments, err := c.cs.ListConsignments(ctx, filter)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to retrieve consignments", err)
		return
	}
	httputil.JSON(w, http.StatusOK, consignments)
}

// HandleGetConsignmentByID handles GET /api/v1/consignments/{id}.
func (c *Router) HandleGetConsignmentByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	consignmentID := r.PathValue("id")
	if consignmentID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}

	// Resolve the caller's company. Fail closed on any identity problem: a missing
	// company profile or an unusable OU handle must not grant access.
	userCompany, err := c.company.GetCompanyByOUHandle(ctx, authCtx.User.OUHandle)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) || errors.Is(err, company.ErrInvalidCompanyID) {
			httputil.Error(w, r, http.StatusForbidden, errCompanyNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to resolve user company", err, "ouHandle", authCtx.User.OUHandle)
		return
	}

	// Fetch the consignment scoped to the caller's company and JWT role. GetConsignmentByID
	// enforces role-tied ownership on the single row read and returns ErrAccessDenied for a
	// cross-company or wrong-role caller before doing any workflow-engine or task-store work.
	consignment, err := c.cs.GetConsignmentByID(ctx, consignmentID, userCompany.ID, authCtx.User.Roles)
	if err != nil {
		switch {
		case errors.Is(err, ErrAccessDenied):
			c.audit.Record(ctx, nswaudit.Event{
				EventType:  nswaudit.EventConsignment,
				Action:     nswaudit.ActionRead,
				TargetType: nswaudit.TargetConsignment,
				TargetID:   consignmentID,
				Failure:    true,
				Metadata: map[string]any{
					"error":           "consignment access denied",
					"callerCompanyId": userCompany.ID,
				},
			})
			// Respond with ErrConsignmentNotFound's text, not ErrAccessDenied's, and
			// 404 (not 403), so a denied read — whether cross-company or company-matched
			// with the wrong role — is indistinguishable from a non-existent consignment
			// and cannot be used to probe which IDs exist.
			httputil.Error(w, r, http.StatusNotFound, errConsignmentNotFound)
			return
		case errors.Is(err, ErrConsignmentNotFound):
			httputil.Error(w, r, http.StatusNotFound, errConsignmentNotFound)
			return
		default:
			httputil.InternalServerError(w, r, "failed to retrieve consignment", err)
			return
		}
	}

	httputil.JSON(w, http.StatusOK, consignment)
}

// HandleAdminGetConsignmentByID handles GET /api/v1/admin/consignments/{id}. Ops/admin callers
// holding ConsignmentAdminRead (enforced at the route, see bootstrap/app.go) may fetch the full
// consignment detail for any consignment, with no trader/CHA ownership check — unlike
// HandleGetConsignmentByID above, which is scoped to the caller's own trader/CHA-owned
// consignments.
func (c *Router) HandleAdminGetConsignmentByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	consignmentID := r.PathValue("id")
	if consignmentID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}

	consignment, err := c.cs.GetConsignmentByIDForAdmin(ctx, consignmentID)
	if err != nil {
		if errors.Is(err, ErrConsignmentNotFound) {
			httputil.Error(w, r, http.StatusNotFound, errConsignmentNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to retrieve consignment", err)
		return
	}

	c.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventConsignment,
		Action:     nswaudit.ActionRead,
		TargetType: nswaudit.TargetConsignment,
		TargetID:   consignmentID,
		Metadata:   map[string]any{"view": "admin"},
	})
	httputil.JSON(w, http.StatusOK, consignment)
}

// HandleGetConsignmentAgency handles GET /api/v1/consignments/{id}/agency.
// Authenticated M2M (or user) callers with nsw:consignment:read may fetch the
// allowlisted display names. Knowing the unguessable UUID is sufficient; there
// is no trader/CHA company ownership check. Access is audit-logged.
func (c *Router) HandleGetConsignmentAgency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.Type() == "" {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	consignmentID := r.PathValue("id")
	if consignmentID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}

	dto, err := c.cs.GetAgencySummary(ctx, consignmentID)
	if err != nil {
		if errors.Is(err, ErrConsignmentNotFound) {
			c.audit.Record(ctx, nswaudit.Event{
				EventType:  nswaudit.EventConsignment,
				Action:     nswaudit.ActionRead,
				TargetType: nswaudit.TargetConsignment,
				TargetID:   consignmentID,
				Failure:    true,
				Metadata: map[string]any{
					"view":  "agency",
					"error": errConsignmentNotFound,
				},
			})
			httputil.Error(w, r, http.StatusNotFound, errConsignmentNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to retrieve consignment agency summary", err)
		return
	}

	c.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventConsignment,
		Action:     nswaudit.ActionRead,
		TargetType: nswaudit.TargetConsignment,
		TargetID:   consignmentID,
		Failure:    false,
		Metadata: map[string]any{
			"view": "agency",
		},
	})
	httputil.JSON(w, http.StatusOK, dto)
}

// HandleGetConsignmentEngineStatus handles GET /api/v1/admin/consignments/{id}/engine-status.
// Returns the root workflow's raw engine state (per-node status straight from the workflow
// manager, e.g. RUNNING/COMPLETED/AWAITING_ADMIN) — an ops/admin view distinct from the
// trader-facing GetConsignmentByID, which reflects task-store/business state instead.
//
// Gated on scopes.ConsignmentAdminRead at the route (see bootstrap/app.go), not on the
// trader/CHA nsw:consignment:read scope — this performs no per-consignment ownership
// check, so any caller holding the admin scope can view any consignment's engine state
// by design.
func (c *Router) HandleGetConsignmentEngineStatus(w http.ResponseWriter, r *http.Request) {
	c.handleEngineStatus(w, r, "engine-status", c.cs.GetEngineStatus)
}

// HandleGetTaskWorkflowEngineStatus handles GET /api/v1/admin/task/{id}/engine-status.
// {id} is a task workflow's own workflow ID (see EngineNodeDTO.TaskWorkflowID, surfaced by
// HandleGetConsignmentEngineStatus on the TASK node that spawned it) — a separate ID space and
// workflow.Manager from the consignment/child-workflow IDs HandleGetConsignmentEngineStatus
// queries. Gated on scopes.ConsignmentAdminRead at the route (see bootstrap/app.go).
func (c *Router) HandleGetTaskWorkflowEngineStatus(w http.ResponseWriter, r *http.Request) {
	c.handleEngineStatus(w, r, "task-workflow-engine-status", c.cs.GetTaskWorkflowEngineStatus)
}

// handleEngineStatus is the common request/response handling shared by
// HandleGetConsignmentEngineStatus and HandleGetTaskWorkflowEngineStatus — they differ only in
// which Service method resolves {id} into an *EngineStatusDTO and the audit "view" label.
func (c *Router) handleEngineStatus(
	w http.ResponseWriter, r *http.Request,
	view string,
	fetch func(ctx context.Context, id string) (*EngineStatusDTO, error),
) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.Type() == "" {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}

	status, err := fetch(ctx, id)
	if err != nil {
		if errors.Is(err, ErrEngineWorkflowNotFound) {
			httputil.Error(w, r, http.StatusNotFound, errWorkflowNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to retrieve "+view, err)
		return
	}

	// TODO(#477): for HandleGetTaskWorkflowEngineStatus, id is a Temporal task-workflow ID
	// (e.g. "task-wf-n1_apply:..."), not a consignment ID — this mislabels TargetID under
	// TargetConsignment. Fixing it needs a TaskStore lookup (task-workflow ID ->
	// TaskRecord.RootWorkflowID) that doesn't exist yet; see the issue for why the obvious
	// GlobalVariables[VarRootWorkflowID] shortcut doesn't work here.
	c.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventConsignment,
		Action:     nswaudit.ActionRead,
		TargetType: nswaudit.TargetConsignment,
		TargetID:   id,
		Failure:    false,
		Metadata: map[string]any{
			"view": view,
		},
	})
	httputil.JSON(w, http.StatusOK, status)
}

// ResolveAdminInterventionRequest is the request body for HandleResolveAdminIntervention.
type ResolveAdminInterventionRequest struct {
	Action    string         `json:"action"`
	Overrides map[string]any `json:"overrides,omitempty"`
	Reason    string         `json:"reason"`
}

// HandleResolveAdminIntervention handles POST /api/v1/admin/consignments/{id}/nodes/{nodeId}/resolve.
// {id} is the ID of the workflow instance containing the node: the consignment's root workflow, a
// child-branch workflow (a node's child_workflow_ids) or a task workflow (a node's
// task_workflow_id). It is not a consignment record ID. {nodeId} is the node's composite ID from
// that workflow's engine status.
//
// Requires scopes.ConsignmentAdminWrite (see bootstrap/app.go): resolving can change workflow data
// (Overrides) or force a path the interpreter didn't choose (Skip/Abort).
func (c *Router) HandleResolveAdminIntervention(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.Type() == "" {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	workflowID := r.PathValue("id")
	nodeID := r.PathValue("nodeId")
	if workflowID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}
	if nodeID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errNodeIDRequired)
		return
	}

	var req ResolveAdminInterventionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestBody)
		return
	}
	action, ok := validAdminActions[req.Action]
	if !ok {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidAdminAction)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		httputil.Error(w, r, http.StatusBadRequest, errReasonRequired)
		return
	}

	auditFail := func(errMsg string) {
		c.audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventConsignment,
			Action:     nswaudit.ActionUpdate,
			TargetType: nswaudit.TargetConsignment,
			TargetID:   workflowID,
			Failure:    true,
			Metadata: map[string]any{
				"view":   "admin-resolve",
				"nodeId": nodeID,
				"action": req.Action,
				"error":  errMsg,
			},
		})
	}

	sig := workflow.AdminResolutionSignal{
		NodeID:    nodeID,
		Action:    action,
		Overrides: req.Overrides,
		Reason:    req.Reason,
	}
	if err := c.cs.ResolveAdminIntervention(ctx, workflowID, sig); err != nil {
		switch {
		case errors.Is(err, ErrEngineWorkflowNotFound):
			auditFail(errWorkflowNotFound)
			httputil.Error(w, r, http.StatusNotFound, errWorkflowNotFound)
		case errors.Is(err, ErrNodeNotParked):
			auditFail(err.Error())
			httputil.Error(w, r, http.StatusConflict, err.Error())
		case errors.Is(err, ErrAdminActionUnsupportedForGateway):
			auditFail(err.Error())
			httputil.Error(w, r, http.StatusBadRequest, err.Error())
		default:
			auditFail(err.Error())
			httputil.InternalServerError(w, r, "failed to resolve admin intervention", err)
		}
		return
	}

	c.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventConsignment,
		Action:     nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetConsignment,
		TargetID:   workflowID,
		Failure:    false,
		Metadata: map[string]any{
			"view":   "admin-resolve",
			"nodeId": nodeID,
			"action": req.Action,
			"reason": req.Reason,
		},
	})
	w.WriteHeader(http.StatusNoContent)
}
