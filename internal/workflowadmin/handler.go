package workflowadmin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/httputil"
	workflow "github.com/OpenNSW/core/workflow"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

const (
	errUnauthorized       = "unauthorized"
	errWorkflowIDRequired = "workflow ID is required"
	errWorkflowNotFound   = "workflow execution not found"
	errStepIDRequired     = "step ID is required"
	errInvalidRequestBody = "invalid request body"
	errInvalidAdminAction = "action must be one of RETRY, COMPLETE, ABORT"
	errReasonRequired     = "reason is required"
	errCaseIDRequired     = "case ID is required"
	errCaseNotFound       = "case not found"
)

// validAdminActions maps the wire-format action string onto the core/workflow constant, and
// doubles as the allowlist HandleResolveAdminIntervention validates against.
var validAdminActions = map[string]workflow.AdminResolutionAction{
	string(workflow.AdminActionRetry):    workflow.AdminActionRetry,
	string(workflow.AdminActionComplete): workflow.AdminActionComplete,
	string(workflow.AdminActionAbort):    workflow.AdminActionAbort,
}

// Handler serves the admin routes on svc and cases, auditing every read and override.
type Handler struct {
	svc   *Service
	cases *CaseService
	audit nswaudit.Auditor
}

// NewHandler creates a Handler. A nil auditor records nothing.
func NewHandler(svc *Service, cases *CaseService, auditor nswaudit.Auditor) *Handler {
	return &Handler{svc: svc, cases: cases, audit: auditor}
}

// HandleGetCase handles GET /api/v1/admin/cases/{id}: any case, with no ownership
// check — a consignment's or an agency case's, addressed by the one ID space they share.
// Every task on it is listed whatever its read roles, and its flows by workflow ID, which
// an admin opens on the routes above.
//
// Gated on scopes.ConsignmentAdminRead at the route (see bootstrap/app.go), the same scope
// as the engine-status routes.
func (h *Handler) HandleGetCase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authn.FromContext(ctx); !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		httputil.Error(w, r, http.StatusBadRequest, errCaseIDRequired)
		return
	}

	detail, err := h.cases.GetCaseDetail(ctx, id)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) {
			httputil.Error(w, r, http.StatusNotFound, errCaseNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to retrieve case", err)
		return
	}
	h.auditEvent(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, id, false, map[string]any{"view": "admin-case"}))
	httputil.JSON(w, http.StatusOK, detail)
}

// HandleGetWorkflowEngineStatus handles GET /api/v1/admin/workflows/{id}/engine-status.
// {id} is a workflow ID: one of a case's flows (see the flows of the
// admin consignment and case views) or a child workflow of one (a node's child_workflow_ids).
// Returns that workflow's raw engine state (per-node status straight from the workflow
// manager, e.g. RUNNING/COMPLETED/AWAITING_ADMIN) — an ops/admin view distinct from the
// trader-facing GetConsignmentByID, which reflects task-store/business state instead.
//
// Gated on scopes.ConsignmentAdminRead at the route (see bootstrap/app.go), not on the
// trader/CHA consignment:read scope — this performs no per-case ownership
// check, so any caller holding the admin scope can view any workflow's engine state
// by design.
func (h *Handler) HandleGetWorkflowEngineStatus(w http.ResponseWriter, r *http.Request) {
	h.handleEngineStatus(w, r, "engine-status", true, h.svc.GetEngineStatus)
}

// HandleGetTaskWorkflowEngineStatus handles GET /api/v1/admin/task/{id}/engine-status.
// {id} is a task workflow's own workflow ID (see EngineNodeDTO.TaskWorkflowID, surfaced by
// HandleGetWorkflowEngineStatus on the TASK node that spawned it) — a separate ID space and
// workflow.Manager from the flow/child-workflow IDs HandleGetWorkflowEngineStatus
// queries. Gated on scopes.ConsignmentAdminRead at the route (see bootstrap/app.go).
func (h *Handler) HandleGetTaskWorkflowEngineStatus(w http.ResponseWriter, r *http.Request) {
	h.handleEngineStatus(w, r, "task-workflow-engine-status", false, h.svc.GetTaskWorkflowEngineStatus)
}

// handleEngineStatus is the common request/response handling shared by
// HandleGetWorkflowEngineStatus and HandleGetTaskWorkflowEngineStatus — they differ only in
// which Service method resolves {id} into an *EngineStatusDTO, the audit "view" label, and
// whether the target comes from the flow variables.
func (h *Handler) handleEngineStatus(
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
		httputil.Error(w, r, http.StatusBadRequest, errWorkflowIDRequired)
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

	event := nswaudit.ConsignmentEvent(nswaudit.ActionRead, id, false, map[string]any{"view": view})
	if flowAudit {
		caseID, _ := status.GlobalVariables[workflow.VarCaseID].(string)
		h.auditFlowEventWithCaseID(ctx, event, caseID)
	} else {
		h.auditTaskWorkflowEvent(ctx, event)
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
// {id} is the ID of the workflow instance containing the node: one of a case's flows or a
// child-branch workflow (a node's child_workflow_ids). {stepId}
// is the parked node's step_id from that workflow's engine status: it names that one parking, so a
// resolve sent after the node was resolved and parked again is rejected (409). A node inside a
// task workflow is resolved through HandleResolveTaskWorkflowAdminIntervention instead.
//
// Requires scopes.ConsignmentAdminWrite (see bootstrap/app.go): resolving can change workflow data
// (GlobalVariablesPatch) or force a path the interpreter didn't choose (Complete/Abort).
func (h *Handler) HandleResolveAdminIntervention(w http.ResponseWriter, r *http.Request) {
	h.handleResolveAdminIntervention(w, r, "admin-resolve", h.svc.ResolveAdminIntervention, h.auditFlowEvent)
}

// HandleResolveTaskWorkflowAdminIntervention handles
// POST /api/v1/admin/task/{id}/steps/{stepId}/resolve. {id} is a task workflow's own workflow ID
// (a TASK node's task_workflow_id) — a separate ID space and workflow.Manager from the
// flow/child-workflow IDs HandleResolveAdminIntervention takes, the same split as the two
// engine-status routes. Otherwise identical, and gated on the same scope.
func (h *Handler) HandleResolveTaskWorkflowAdminIntervention(w http.ResponseWriter, r *http.Request) {
	h.handleResolveAdminIntervention(w, r, "task-workflow-admin-resolve", h.svc.ResolveTaskWorkflowAdminIntervention, h.auditTaskWorkflowEvent)
}

// handleResolveAdminIntervention is the request handling the two resolve routes share; they differ
// in which Service method resolves {id}, the audit "view" label, and the audit target resolver.
func (h *Handler) handleResolveAdminIntervention(
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
		httputil.Error(w, r, http.StatusBadRequest, errWorkflowIDRequired)
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
// {id} is the workflow instance containing the node, one of a case's flows or a
// child-branch workflow, and {stepId} is the node's step_id from that workflow's engine status. The
// node must be a TASK waiting on its task; the task is closed and the node moved to AWAITING_ADMIN
// (see Service.ForceParkTaskNode), after which the resolve route applies.
//
// Requires scopes.ConsignmentAdminWrite, like resolve: it takes a task away from the trader or
// officer working on it.
func (h *Handler) HandleForceParkTaskNode(w http.ResponseWriter, r *http.Request) {
	h.handleForcePark(w, r, "admin-force-park", h.svc.ForceParkTaskNode, h.auditFlowEvent)
}

// HandleForceParkTaskWorkflowStep handles POST /api/v1/admin/task/{id}/steps/{stepId}/force-park.
// {id} is a task workflow's own workflow ID (a TASK node's task_workflow_id) and {stepId} is one of
// its steps. The step is moved to AWAITING_ADMIN and the task stays open (see
// Service.ForceParkTaskWorkflowStep); the task workflow's resolve route applies after. Gated on the
// same scope as HandleForceParkTaskNode.
func (h *Handler) HandleForceParkTaskWorkflowStep(w http.ResponseWriter, r *http.Request) {
	h.handleForcePark(w, r, "task-workflow-admin-force-park", h.svc.ForceParkTaskWorkflowStep, h.auditTaskWorkflowEvent)
}

// handleForcePark is the request handling the two force-park routes share; they differ only in
// which Service method parks the step, the audit "view" label, and the audit target resolver.
func (h *Handler) handleForcePark(
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
		httputil.Error(w, r, http.StatusBadRequest, errWorkflowIDRequired)
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

// auditTaskWorkflowEvent resolves a task workflow's case from its persisted task record.
// Task-workflow variables do not reliably carry the case ID. A missing record leaves
// the target empty while retaining the workflow ID and the operation's original result.
func (h *Handler) auditTaskWorkflowEvent(ctx context.Context, e nswaudit.Event) {
	if h.audit == nil {
		return
	}
	caseID := ""
	if h.svc.taskStore != nil {
		if record, ok := h.svc.taskStore.GetTaskByWorkflowID(ctx, e.TargetID); ok {
			caseID = record.CaseID
		}
	}
	h.auditFlowEventWithCaseID(ctx, e, caseID)
}

// auditFlowEvent resolves the case for a flow/child-workflow mutation audit. The extra
// status read is best-effort: the mutation's result must survive a lookup failure or a
// workflow that has already closed. Engine-status reads reuse their existing snapshot instead.
func (h *Handler) auditFlowEvent(ctx context.Context, e nswaudit.Event) {
	if h.audit == nil {
		return
	}
	caseID := ""
	if h.svc.wm != nil {
		if instance, err := h.svc.wm.GetStatus(ctx, e.TargetID); err == nil && instance != nil {
			caseID, _ = instance.WorkflowVariables[workflow.VarCaseID].(string)
		}
	}
	h.auditFlowEventWithCaseID(ctx, e, caseID)
}

// auditFlowEventWithCaseID keeps a flow or task-workflow address in metadata and targets the case.
// An unknown case stays empty rather than being mislabeled with a flow/child-workflow ID.
func (h *Handler) auditFlowEventWithCaseID(ctx context.Context, e nswaudit.Event, caseID string) {
	if h.audit == nil {
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
	h.auditEvent(ctx, e)
}

func (h *Handler) auditEvent(ctx context.Context, e nswaudit.Event) {
	if h.audit != nil {
		h.audit.Audit(ctx, e)
	}
}
