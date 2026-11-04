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
	errInvalidRequestBody    = "invalid request body"
	errFlowNotAllowed        = "templateId is not a flow that can be started on this consignment"
	errRequestBodyTooLarge   = "request body too large"
)

// maxStartFlowRequestBytes caps a StartFlowRequest body, which only names a template.
const maxStartFlowRequestBytes = 1 << 10

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
// Authenticated M2M (or user) callers with consignment:read may fetch the
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

func (c *Router) auditEvent(ctx context.Context, e nswaudit.Event) {
	if c.audit != nil {
		c.audit.Audit(ctx, e)
	}
}
