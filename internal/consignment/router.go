package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/pagination"
	workflow "github.com/OpenNSW/core/workflow"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

const (
	errUnauthorized          = "unauthorized"
	errConsignmentIDRequired = "consignment ID is required"
	errInvalidState          = "query param state must be IN_PROGRESS or FINISHED"
	errInvalidFlow           = "query param flow must be IMPORT or EXPORT"
	errForbiddenRole         = "caller holds neither the trader nor the cha role"
	errTraderRoleRequired    = "creating a consignment requires the trader role"
	errCompanyNotFound       = "company not found"
	errConsignmentNotFound   = "consignment not found"
	errWorkflowNotFound      = "workflow execution not found"
	errStepIDRequired        = "step ID is required"
	errInvalidRequestBody    = "invalid request body"
	errInvalidAdminAction    = "action must be one of RETRY, COMPLETE, ABORT"
	errReasonRequired        = "reason is required"
	errFlowNotAllowed        = "templateId is not a flow that can be started on this consignment"
	errRequestBodyTooLarge   = "request body too large"
)

// maxStartFlowRequestBytes caps a StartFlowRequest body, which only names a template.
const maxStartFlowRequestBytes = 1 << 10

// validAdminActions maps the wire-format action string onto the core/workflow constant, and
// doubles as the allowlist HandleResolveAdminIntervention validates against.
var validAdminActions = map[string]workflow.AdminResolutionAction{
	string(workflow.AdminActionRetry):    workflow.AdminActionRetry,
	string(workflow.AdminActionComplete): workflow.AdminActionComplete,
	string(workflow.AdminActionAbort):    workflow.AdminActionAbort,
}

type Router struct {
	cs      *Service
	cha     cha.Service
	company company.Service
	audit   nswaudit.Auditor
	roles   map[string]string // logical name ("trader"/"cha") -> IdP token role
}

// NewRouter builds the router. roles is the global catalog's Roles map; it must
// define "trader" and "cha" — the handlers resolve the caller's token roles
// through it.
func NewRouter(cs *Service, chaService cha.Service, companyService company.Service, auditor nswaudit.Auditor, roles map[string]string) (*Router, error) {
	if err := validateRoles(roles); err != nil {
		return nil, err
	}
	return &Router{cs: cs, cha: chaService, company: companyService, audit: auditor, roles: roles}, nil
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
const defaultExportWorkflowTemplateID = "trade-export"

// HandleCreateConsignment handles POST /api/v1/consignments
// Creates an export consignment and starts its workflow directly — no CHA company or HS code
// is collected up front; the workflow's own tasks collect those later. Response: DetailDTO.
func (c *Router) HandleCreateConsignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := authn.FromContext(ctx)
	if !ok || p.Kind != authn.KindUser {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if !slices.Contains(p.Roles, c.roles["trader"]) {
		c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionCreate, "", true, map[string]any{
			"error": "trader role not held",
		}))
		httputil.Error(w, r, http.StatusForbidden, errTraderRoleRequired)
		return
	}

	traderID := p.UserID
	consignment, err := c.cs.CreateAndStartConsignment(ctx, traderID, defaultExportWorkflowTemplateID)
	if err != nil {
		c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionCreate, "", true, map[string]any{
			"error": err.Error(),
		}))
		httputil.InternalServerError(w, r, "failed to create and start consignment", err)
		return
	}
	if consignment == nil {
		c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionCreate, "", true, map[string]any{
			"error": "consignment is nil after successful creation",
		}))
		httputil.InternalServerError(w, r, "consignment is nil after successful creation", nil)
		return
	}

	c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionCreate, consignment.ID, false, map[string]any{
		"flow":            consignment.Flow,
		"traderCompanyId": consignment.TraderCompanyID,
		"chaCompanyId":    consignment.ChaCompanyID,
	}))
	httputil.JSON(w, http.StatusCreated, consignment)
}

// buildConsignmentFilter parses pagination and the optional query filters (state, flow, q)
// from the request. Invalid pagination, or an unknown state or flow, returns the
// client-facing error message to send with a 400.
func buildConsignmentFilter(r *http.Request) (Filter, string) {
	offset, limit, err := pagination.ParsePaginationParams(r)
	if err != nil {
		slog.WarnContext(r.Context(), "invalid pagination parameters", "error", err)
		return Filter{}, "invalid pagination parameters"
	}
	filter := Filter{Offset: offset, Limit: limit}
	if stateStr := r.URL.Query().Get("state"); stateStr != "" {
		state := State(stateStr)
		if !state.Valid() {
			return Filter{}, errInvalidState
		}
		filter.State = &state
	}
	if flowStr := r.URL.Query().Get("flow"); flowStr != "" {
		flow := Flow(flowStr)
		if !flow.Valid() {
			return Filter{}, errInvalidFlow
		}
		filter.Flow = &flow
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		filter.Query = &q
	}
	return filter, ""
}

// HandleGetConsignments handles GET /api/v1/consignments
// The scope comes from the caller's token roles: the trader role lists the consignments
// of the caller's company as trader, the cha role those it is the CHA on, and holding
// both lists both. A caller holding neither is forbidden.
func (c *Router) HandleGetConsignments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := authn.FromContext(ctx)
	if !ok || p.Kind != authn.KindUser {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}

	filter, filterErr := buildConsignmentFilter(r)
	if filterErr != "" {
		httputil.Error(w, r, http.StatusBadRequest, filterErr)
		return
	}

	// Resolved through the global catalog, not hardcoded, so it stays in step with the
	// same "trader"/"cha" -> token-role mapping the task-authz layer
	// (internal/tasks/taskauthz) uses.
	asTrader := slices.Contains(p.Roles, c.roles["trader"])
	asCHA := slices.Contains(p.Roles, c.roles["cha"])
	if !asTrader && !asCHA {
		c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, "", true, map[string]any{
			"error": "role not held",
		}))
		httputil.Error(w, r, http.StatusForbidden, errForbiddenRole)
		return
	}

	userCompany, err := c.company.GetCompanyByOUHandle(ctx, p.OUHandle)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) || errors.Is(err, company.ErrInvalidCompanyID) {
			c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, "", true, map[string]any{
				"error": "company not found",
			}))
			httputil.Error(w, r, http.StatusForbidden, errCompanyNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to resolve user company", err, "ouHandle", p.OUHandle)
		return
	}

	if asTrader {
		filter.TraderCompanyID = &userCompany.ID
	}
	if asCHA {
		filter.CHACompanyID = &userCompany.ID
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
	p, ok := authn.FromContext(ctx)
	if !ok || p.Kind != authn.KindUser {
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
	userCompany, err := c.company.GetCompanyByOUHandle(ctx, p.OUHandle)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) || errors.Is(err, company.ErrInvalidCompanyID) {
			httputil.Error(w, r, http.StatusForbidden, errCompanyNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to resolve user company", err, "ouHandle", p.OUHandle)
		return
	}

	// Fetch the consignment scoped to the caller's company and JWT role. GetConsignmentByID
	// enforces role-tied ownership on the single row read and returns ErrAccessDenied for a
	// cross-company or wrong-role caller before doing any workflow-engine or task-store work.
	consignment, err := c.cs.GetConsignmentByID(ctx, consignmentID, userCompany.ID, p.Roles)
	if err != nil {
		switch {
		case errors.Is(err, ErrAccessDenied):
			c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, consignmentID, true, map[string]any{
				"error":           "consignment access denied",
				"callerCompanyId": userCompany.ID,
			}))
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

	c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, consignmentID, false, map[string]any{
		"callerCompanyId": userCompany.ID,
	}))
	httputil.JSON(w, http.StatusOK, consignment)
}

// HandleStartConsignmentFlow handles POST /api/v1/consignments/{id}/flows. A trader or CHA starts
// one of the consignment's startable flows (DetailDTO.StartableFlows) — e.g. a port or agency fee
// payment — under the same company-and-role check as HandleGetConsignmentByID. Body:
// StartFlowRequest, which names the flow and nothing else: any other field, such as variables
// for the flow, is rejected. Response: 201 with the started flow's FlowDTO.
func (c *Router) HandleStartConsignmentFlow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := authn.FromContext(ctx)
	if !ok || p.Kind != authn.KindUser {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	consignmentID := r.PathValue("id")
	if consignmentID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}

	var req StartFlowRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxStartFlowRequestBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&req)
	if err == nil {
		// One JSON value only: anything after it is rejected too.
		if err = dec.Decode(new(struct{})); errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			err = errors.New("unexpected data after JSON value")
		}
	}
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httputil.Error(w, r, http.StatusRequestEntityTooLarge, errRequestBodyTooLarge)
			return
		}
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestBody)
		return
	}

	// Resolve the caller's company. Fail closed on any identity problem, as
	// HandleGetConsignmentByID does.
	userCompany, err := c.company.GetCompanyByOUHandle(ctx, p.OUHandle)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) || errors.Is(err, company.ErrInvalidCompanyID) {
			httputil.Error(w, r, http.StatusForbidden, errCompanyNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to resolve user company", err, "ouHandle", p.OUHandle)
		return
	}

	auditEvent := func(failure bool, metadata map[string]any) {
		metadata["view"] = "start-flow"
		metadata["templateId"] = req.TemplateID
		c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionUpdate, consignmentID, failure, metadata))
	}

	flow, err := c.cs.StartConsignmentFlow(ctx, consignmentID, req.TemplateID, userCompany.ID, p.Roles)
	if err != nil {
		auditEvent(true, map[string]any{"error": err.Error(), "callerCompanyId": userCompany.ID})
		switch {
		case errors.Is(err, ErrFlowNotAllowed):
			httputil.Error(w, r, http.StatusBadRequest, errFlowNotAllowed)
		case errors.Is(err, ErrAccessDenied), errors.Is(err, ErrConsignmentNotFound):
			// 404 for a denied caller too, as HandleGetConsignmentByID does, so the
			// response cannot be used to probe which IDs exist.
			httputil.Error(w, r, http.StatusNotFound, errConsignmentNotFound)
		default:
			httputil.InternalServerError(w, r, "failed to start flow", err)
		}
		return
	}

	auditEvent(false, map[string]any{"workflowId": flow.WorkflowID})
	httputil.JSON(w, http.StatusCreated, flow)
}

// HandleAdminGetConsignmentByID handles GET /api/v1/admin/consignments/{id}. Ops/admin callers
// holding ConsignmentAdminRead (enforced at the route, see bootstrap/app.go) may fetch the full
// consignment detail for any consignment, with no trader/CHA ownership check — unlike
// HandleGetConsignmentByID above, which is scoped to the caller's own trader/CHA-owned
// consignments.
func (c *Router) HandleAdminGetConsignmentByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := authn.FromContext(ctx)
	if !ok || p.Kind != authn.KindUser {
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

	c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, consignmentID, false, map[string]any{"view": "admin"}))
	httputil.JSON(w, http.StatusOK, consignment)
}

// HandleGetConsignmentAgency handles GET /api/v1/consignments/{id}/agency.
// Authenticated M2M (or user) callers with nsw:consignment:read may fetch the
// allowlisted display names. Knowing the unguessable UUID is sufficient; there
// is no trader/CHA company ownership check. Access is audit-logged.
func (c *Router) HandleGetConsignmentAgency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authn.FromContext(ctx); !ok {
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
			c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, consignmentID, true, map[string]any{
				"view":  "agency",
				"error": errConsignmentNotFound,
			}))
			httputil.Error(w, r, http.StatusNotFound, errConsignmentNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to retrieve consignment agency summary", err)
		return
	}

	c.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, consignmentID, false, map[string]any{
		"view": "agency",
	}))
	httputil.JSON(w, http.StatusOK, dto)
}

// HandleGetWorkflowEngineStatus handles GET /api/v1/admin/workflows/{id}/engine-status.
// {id} is a workflow ID: one of a consignment's flows (see the flows of
// HandleAdminGetConsignmentByID) or a child workflow of one (a node's child_workflow_ids).
// Returns that workflow's raw engine state (per-node status straight from the workflow
// manager, e.g. RUNNING/COMPLETED/AWAITING_ADMIN) — an ops/admin view distinct from the
// trader-facing GetConsignmentByID, which reflects task-store/business state instead.
//
// Gated on scopes.ConsignmentAdminRead at the route (see bootstrap/app.go), not on the
// trader/CHA nsw:consignment:read scope — this performs no per-consignment ownership
// check, so any caller holding the admin scope can view any workflow's engine state
// by design.
func (c *Router) HandleGetWorkflowEngineStatus(w http.ResponseWriter, r *http.Request) {
	c.handleEngineStatus(w, r, "engine-status", true, c.cs.GetEngineStatus)
}

// HandleGetTaskWorkflowEngineStatus handles GET /api/v1/admin/task/{id}/engine-status.
// {id} is a task workflow's own workflow ID (see EngineNodeDTO.TaskWorkflowID, surfaced by
// HandleGetWorkflowEngineStatus on the TASK node that spawned it) — a separate ID space and
// workflow.Manager from the flow/child-workflow IDs HandleGetWorkflowEngineStatus
// queries. Gated on scopes.ConsignmentAdminRead at the route (see bootstrap/app.go).
func (c *Router) HandleGetTaskWorkflowEngineStatus(w http.ResponseWriter, r *http.Request) {
	c.handleEngineStatus(w, r, "task-workflow-engine-status", false, c.cs.GetTaskWorkflowEngineStatus)
}

// handleEngineStatus is the common request/response handling shared by
// HandleGetWorkflowEngineStatus and HandleGetTaskWorkflowEngineStatus — they differ only in
// which Service method resolves {id} into an *EngineStatusDTO, the audit "view" label, and
// whether the target comes from the flow variables.
func (c *Router) handleEngineStatus(
	w http.ResponseWriter, r *http.Request,
	view string,
	flowAudit bool,
	fetch func(ctx context.Context, id string) (*EngineStatusDTO, error),
) {
	ctx := r.Context()
	if _, ok := authn.FromContext(ctx); !ok {
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
	// TaskRecord.CaseID) that doesn't exist yet; see the issue for why the obvious
	// GlobalVariables[VarCaseID] shortcut doesn't work here.
	event := nswaudit.ConsignmentEvent(nswaudit.ActionRead, id, false, map[string]any{"view": view})
	if flowAudit {
		caseID, _ := status.GlobalVariables[workflow.VarCaseID].(string)
		c.auditFlowEventWithCaseID(ctx, event, caseID)
	} else {
		c.auditEvent(ctx, event)
	}
	httputil.JSON(w, http.StatusOK, status)
}

// ResolveAdminInterventionRequest is the request body for HandleResolveAdminIntervention.
//
// GlobalVariablesPatch is a patch, not the full variable set: dotted paths (e.g.
// "review.outcome") mapped to the values to write, applied before RETRY re-runs the node or
// COMPLETE marks it done. A map value is merged into an existing map at that path and any other
// value replaces it. Variables it doesn't name are untouched, and what it writes is
// workflow-wide, so it affects later nodes too.
type ResolveAdminInterventionRequest struct {
	Action               string         `json:"action"`
	GlobalVariablesPatch map[string]any `json:"global_variables_patch,omitempty"`
	Reason               string         `json:"reason"`
}

// HandleResolveAdminIntervention handles POST /api/v1/admin/workflows/{id}/steps/{stepId}/resolve.
// {id} is the ID of the workflow instance containing the node: one of a consignment's flows or a
// child-branch workflow (a node's child_workflow_ids). {stepId}
// is the parked node's step_id from that workflow's engine status: it names that one parking, so a
// resolve sent after the node was resolved and parked again is rejected (409). A node inside a
// task workflow is resolved through HandleResolveTaskWorkflowAdminIntervention instead.
//
// Requires scopes.ConsignmentAdminWrite (see bootstrap/app.go): resolving can change workflow data
// (GlobalVariablesPatch) or force a path the interpreter didn't choose (Complete/Abort).
func (c *Router) HandleResolveAdminIntervention(w http.ResponseWriter, r *http.Request) {
	c.handleResolveAdminIntervention(w, r, "admin-resolve", c.cs.ResolveAdminIntervention, c.auditFlowEvent)
}

// HandleResolveTaskWorkflowAdminIntervention handles
// POST /api/v1/admin/task/{id}/steps/{stepId}/resolve. {id} is a task workflow's own workflow ID
// (a TASK node's task_workflow_id) — a separate ID space and workflow.Manager from the
// flow/child-workflow IDs HandleResolveAdminIntervention takes, the same split as the two
// engine-status routes. Otherwise identical, and gated on the same scope.
func (c *Router) HandleResolveTaskWorkflowAdminIntervention(w http.ResponseWriter, r *http.Request) {
	c.handleResolveAdminIntervention(w, r, "task-workflow-admin-resolve", c.cs.ResolveTaskWorkflowAdminIntervention, c.auditEvent)
}

// handleResolveAdminIntervention is the request handling the two resolve routes share; they differ
// in which Service method resolves {id}, the audit "view" label, and the audit target resolver.
func (c *Router) handleResolveAdminIntervention(
	w http.ResponseWriter, r *http.Request,
	view string,
	resolve func(ctx context.Context, workflowID string, sig workflow.AdminResolutionSignal) (nodeID string, err error),
	recordAudit func(context.Context, nswaudit.Event),
) {
	ctx := r.Context()
	if _, ok := authn.FromContext(ctx); !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	workflowID := r.PathValue("id")
	stepID := r.PathValue("stepId")
	if workflowID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}
	if stepID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errStepIDRequired)
		return
	}

	var req ResolveAdminInterventionRequest
	// Unknown fields are rejected rather than ignored: a patch sent under a wrong or old key (e.g.
	// the pre-rename "overrides") would otherwise be dropped while the action still succeeded.
	// Decode only parses the first JSON value, so a second Decode is needed to reject a body
	// with anything trailing it (e.g. two concatenated objects).
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		msg := errInvalidRequestBody
		// encoding/json has no typed error for this, and naming the field is what tells the
		// caller what to fix.
		if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			msg += ": unknown field " + field
		}
		httputil.Error(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
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
		recordAudit(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionUpdate, workflowID, true, map[string]any{
			"view":   view,
			"stepId": stepID,
			"action": req.Action,
			"error":  errMsg,
		}))
	}

	sig := workflow.AdminResolutionSignal{
		ActivationID:           stepID,
		Action:                 action,
		WorkflowVariablesPatch: req.GlobalVariablesPatch,
		Reason:                 req.Reason,
	}
	nodeID, err := resolve(ctx, workflowID, sig)
	if err != nil {
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

	recordAudit(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionUpdate, workflowID, false, map[string]any{
		"view":   view,
		"stepId": stepID,
		"nodeId": nodeID,
		"action": req.Action,
		"reason": req.Reason,
	}))
	w.WriteHeader(http.StatusNoContent)
}

// ForceParkRequest is the request body for HandleForceParkTaskNode.
type ForceParkRequest struct {
	Reason string `json:"reason"`
}

// HandleForceParkTaskNode handles POST /api/v1/admin/workflows/{id}/steps/{stepId}/force-park.
// {id} is the workflow instance containing the node, one of a consignment's flows or a
// child-branch workflow, and {stepId} is the node's step_id from that workflow's engine status. The
// node must be a TASK waiting on its task; the task is closed and the node moved to AWAITING_ADMIN
// (see Service.ForceParkTaskNode), after which the resolve route applies.
//
// Requires scopes.ConsignmentAdminWrite, like resolve: it takes a task away from the trader or
// officer working on it.
func (c *Router) HandleForceParkTaskNode(w http.ResponseWriter, r *http.Request) {
	c.handleForcePark(w, r, "admin-force-park", c.cs.ForceParkTaskNode, c.auditFlowEvent)
}

// HandleForceParkTaskWorkflowStep handles POST /api/v1/admin/task/{id}/steps/{stepId}/force-park.
// {id} is a task workflow's own workflow ID (a TASK node's task_workflow_id) and {stepId} is one of
// its steps. The step is moved to AWAITING_ADMIN and the task stays open (see
// Service.ForceParkTaskWorkflowStep); the task workflow's resolve route applies after. Gated on the
// same scope as HandleForceParkTaskNode.
func (c *Router) HandleForceParkTaskWorkflowStep(w http.ResponseWriter, r *http.Request) {
	c.handleForcePark(w, r, "task-workflow-admin-force-park", c.cs.ForceParkTaskWorkflowStep, c.auditEvent)
}

// handleForcePark is the request handling the two force-park routes share; they differ only in
// which Service method parks the step, the audit "view" label, and the audit target resolver.
func (c *Router) handleForcePark(
	w http.ResponseWriter, r *http.Request,
	view string,
	forcePark func(ctx context.Context, workflowID, stepID, reason string) (nodeID string, err error),
	recordAudit func(context.Context, nswaudit.Event),
) {
	ctx := r.Context()
	if _, ok := authn.FromContext(ctx); !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	workflowID := r.PathValue("id")
	stepID := r.PathValue("stepId")
	if workflowID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDRequired)
		return
	}
	if stepID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errStepIDRequired)
		return
	}

	var req ForceParkRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		msg := errInvalidRequestBody
		if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			msg += ": unknown field " + field
		}
		httputil.Error(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestBody)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		httputil.Error(w, r, http.StatusBadRequest, errReasonRequired)
		return
	}

	auditFail := func(errMsg string) {
		recordAudit(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionUpdate, workflowID, true, map[string]any{
			"view":   view,
			"stepId": stepID,
			"error":  errMsg,
		}))
	}

	nodeID, err := forcePark(ctx, workflowID, stepID, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, ErrEngineWorkflowNotFound):
			auditFail(errWorkflowNotFound)
			httputil.Error(w, r, http.StatusNotFound, errWorkflowNotFound)
		case errors.Is(err, ErrNodeNotWaiting):
			auditFail(err.Error())
			httputil.Error(w, r, http.StatusConflict, ErrNodeNotWaiting.Error())
		case errors.Is(err, ErrForceParkUnsupportedNodeType):
			auditFail(err.Error())
			httputil.Error(w, r, http.StatusBadRequest, err.Error())
		default:
			auditFail(err.Error())
			httputil.InternalServerError(w, r, "failed to force-park node; please retry", err)
		}
		return
	}

	recordAudit(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionUpdate, workflowID, false, map[string]any{
		"view":   view,
		"stepId": stepID,
		"nodeId": nodeID,
		"reason": req.Reason,
	}))
	w.WriteHeader(http.StatusNoContent)
}

// auditFlowEvent resolves the case for a flow/child-workflow mutation audit. The extra
// status read is best-effort: the mutation's result must survive a lookup failure or a
// workflow that has already closed. Engine-status reads reuse their existing snapshot instead.
func (c *Router) auditFlowEvent(ctx context.Context, e nswaudit.Event) {
	if c.audit == nil {
		return
	}
	caseID := ""
	if c.cs.wm != nil {
		if instance, err := c.cs.wm.GetStatus(ctx, e.TargetID); err == nil && instance != nil {
			caseID, _ = instance.WorkflowVariables[workflow.VarCaseID].(string)
		}
	}
	c.auditFlowEventWithCaseID(ctx, e, caseID)
}

// auditFlowEventWithCaseID keeps the workflow address in metadata and targets the case.
// An unknown case stays empty rather than being mislabeled with a flow/child-workflow ID.
func (c *Router) auditFlowEventWithCaseID(ctx context.Context, e nswaudit.Event, caseID string) {
	if c.audit == nil {
		return
	}
	workflowID := e.TargetID
	e.TargetID = caseID
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata["workflowId"] = workflowID
	if caseID == "" {
		e.Metadata["targetResolutionError"] = "workflow consignment could not be resolved"
		slog.WarnContext(ctx, "workflow audit target could not be resolved", "workflowId", workflowID)
	}
	c.auditEvent(ctx, e)
}

func (c *Router) auditEvent(ctx context.Context, e nswaudit.Event) {
	if c.audit != nil {
		c.audit.Audit(ctx, e)
	}
}
