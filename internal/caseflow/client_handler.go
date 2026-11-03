package caseflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/httputil"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// ScopeFlowStart gates POST /api/v1/internal/cases/{caseId}/flows. Starting a client flow
// creates the case if it is new, so only a system trusted to do that holds it. It is an M2M
// permission, kept apart from the officer's task:* scopes.
const ScopeFlowStart = "workflow:inject"

const (
	errInvalidBody     = "invalid request body"
	errWorkflowIDReq   = "workflowId is required"
	errFlowIDReq       = "flowId is required"
	errFlowNotFound    = "flow not found"
	errFailedToStart   = "failed to start flow"
	errFlowIDConflicts = "flowId is already started with different details"
)

// ClientStartRequest is the body of POST /api/v1/internal/cases/{caseId}/flows.
type ClientStartRequest struct {
	// WorkflowID is the workflow definition the flow runs, as flows.clients lists it.
	WorkflowID string `json:"workflowId"`
	// FlowID names the flow, so a repeat of the same request is recognised. It is the
	// workflow instance ID in the engine.
	FlowID string `json:"flowId"`
	// Input is what the caller hands the flow: the application it reviews.
	Input json.RawMessage `json:"input"`
	// CallbackToken is the opaque token the caller expects the decision back on. It names
	// the caller's step, so the flow hands it back unchanged.
	CallbackToken string `json:"callbackToken"`
	// Attributes are values about the case, such as its trader company. A new flow stores
	// them in cases.attributes, and fills the columns flows.clients.caseColumns maps from
	// them.
	Attributes map[string]any `json:"attributes,omitempty"`
}

// ClientHandler serves the route another system starts a flow on: it creates the case if it
// is new, starts the flow, and records this deployment's own OU as an owner of the case.
// Only the flows flows.clients lists for the calling client can be started.
type ClientHandler struct {
	flows   *Service
	list    ClientFlows
	clients map[string]string // OAuth2 client id -> catalog client name
}

// NewClientHandler creates a ClientHandler. clients is the catalog's Clients map (name ->
// client id).
func NewClientHandler(flows *Service, list ClientFlows, clients map[string]string) *ClientHandler {
	byID := make(map[string]string, len(clients))
	for name, id := range clients {
		byID[id] = name
	}
	return &ClientHandler{flows: flows, list: list, clients: byID}
}

// HandleStart handles POST /api/v1/internal/cases/{caseId}/flows. Response: Flow.
//
// A flow the caller may not start answers 404, as one that does not exist would, so a
// caller can't learn which flows are listed for others; the reason is logged.
func (h *ClientHandler) HandleStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caseID := r.PathValue("caseId")
	var req ClientStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidBody)
		return
	}
	switch {
	case req.WorkflowID == "":
		httputil.Error(w, r, http.StatusBadRequest, errWorkflowIDReq)
		return
	case req.FlowID == "":
		httputil.Error(w, r, http.StatusBadRequest, errFlowIDReq)
		return
	}

	flow, ok := h.allowed(ctx, req.WorkflowID, caseID)
	if !ok {
		httputil.Error(w, r, http.StatusNotFound, errFlowNotFound)
		return
	}

	vars, err := clientVars(caseID, req)
	if err != nil {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidBody)
		return
	}
	// The owner is recorded with the flow, so a flow that is recorded is owned, and a
	// retry of a failed start finds both.
	owner := &Owner{Role: flow.Owner, OUHandle: h.list.OUHandle}
	f, err := h.flows.StartFlow(ctx, StartRequest{
		CaseID: caseID, TemplateID: flow.ID, WorkflowID: req.FlowID, Vars: vars, Owner: owner,
		Case: caseInfo(req.Attributes, h.list.CaseColumns),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRequest):
			httputil.Error(w, r, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrConflict):
			httputil.Error(w, r, http.StatusConflict, errFlowIDConflicts)
		default:
			httputil.InternalServerError(w, r, errFailedToStart, err, "caseId", caseID, "flowId", req.FlowID)
		}
		return
	}
	httputil.JSON(w, http.StatusOK, f)
}

// allowed returns the listed flow workflowID names if the calling client may start it. It
// logs why when not.
func (h *ClientHandler) allowed(ctx context.Context, workflowID, caseID string) (ClientFlow, bool) {
	p, ok := authn.FromContext(ctx)
	if !ok || p.Kind != authn.KindClient {
		slog.WarnContext(ctx, "caseflow: client flow refused: caller is not a client", "workflowId", workflowID, "caseId", caseID)
		return ClientFlow{}, false
	}
	client, ok := h.clients[p.ClientID]
	if !ok {
		slog.WarnContext(ctx, "caseflow: client flow refused: client is not in the catalog", "clientId", p.ClientID, "workflowId", workflowID, "caseId", caseID)
		return ClientFlow{}, false
	}
	flow, ok := h.list.Find(workflowID, client)
	if !ok {
		reason := "workflow is not listed for this client"
		if !h.list.Enabled {
			reason = "flows.clients is off"
		}
		slog.WarnContext(ctx, "caseflow: client flow refused: "+reason, "client", client, "workflowId", workflowID, "caseId", caseID)
		return ClientFlow{}, false
	}
	return flow, true
}

// caseInfo is the CaseInfo for attrs: the attributes, and the name columns maps from them.
// Nil when attrs is empty. A name that is missing, not a string or blank leaves the case's
// name as it is.
func caseInfo(attrs map[string]any, columns map[string]string) *CaseInfo {
	if len(attrs) == 0 {
		return nil
	}
	info := &CaseInfo{Attributes: attrs}
	if path, ok := columns[CaseColumnName]; ok {
		if name, ok := attributeAt(attrs, path).(string); ok {
			info.Name = strings.TrimSpace(name)
		}
	}
	return info
}

// attributeAt returns the value at path, dot-separated keys into nested objects, or nil
// when there is none.
func attributeAt(attrs map[string]any, path string) any {
	var v any = attrs
	for _, key := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[key]
	}
	return v
}

// clientVars seeds a client flow's variables under the names the agencies' workflows map
// them by: taskId (the flow ID), taskCode (the workflow), consignmentId (the case), the
// input as notification, and callbackToken when the caller sent one. A workflow that calls
// back by it then fails on the missing variable at the start, not on an empty token.
func clientVars(caseID string, req ClientStartRequest) (map[string]any, error) {
	var input any
	if len(req.Input) > 0 {
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return nil, err
		}
	}
	vars := map[string]any{
		"taskId":        req.FlowID,
		"taskCode":      req.WorkflowID,
		"consignmentId": caseID,
		"notification":  input,
	}
	if req.CallbackToken != "" {
		vars["callbackToken"] = req.CallbackToken
	}
	return vars, nil
}
