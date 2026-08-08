package consignment

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/pagination"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
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
)

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
// "trader" or "cha", or maps either to an empty string. An empty mapping is
// treated as missing: it can never match a real JWT role claim, so honoring it
// would let slices.Contains(callerRoles, "") wrongly succeed if a caller's roles
// ever contained an empty string. Package-private; internal/consignment's
// Service.NewService also calls this (same package, no import needed).
func validateRoles(roles map[string]string) error {
	var missing []string
	for _, name := range []string{"trader", "cha"} {
		if role, ok := roles[name]; !ok || role == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("consignment: catalog is missing required role(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

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
	consignment, err := c.cs.CreateAndStartConsignment(ctx, traderID)
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
	// layer (internal/tasks/extensions/authz) uses.
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
					"error":           "cross-company access denied",
					"callerCompanyId": userCompany.ID,
				},
			})
			// Respond with ErrConsignmentNotFound's text, not ErrAccessDenied's, and
			// 404 (not 403), so a cross-company read is indistinguishable from a
			// non-existent consignment and cannot be used to probe which IDs exist.
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
