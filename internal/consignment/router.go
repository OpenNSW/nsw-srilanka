package consignment

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/pagination"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

type Router struct {
	cs      *Service
	cha     cha.Service
	company company.Service
	audit   *nswaudit.Recorder
}

func NewRouter(cs *Service, chaService cha.Service, companyService company.Service, recorder *nswaudit.Recorder) *Router {
	return &Router{cs: cs, cha: chaService, company: companyService, audit: recorder}
}

// HandleCreateConsignment handles POST /api/v1/consignments
// Creates an export consignment and starts its workflow directly — no CHA company or HS code
// is collected up front; the workflow's own tasks collect those later. Response: DetailDTO.
func (c *Router) HandleCreateConsignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	traderID := authCtx.User.ID
	consignment, err := c.cs.CreateAndStartConsignment(ctx, traderID)
	if err != nil {
		slog.Error("failed to create and start consignment", "error", err)
		c.audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventConsignment,
			Action:     nswaudit.ActionCreate,
			TargetType: nswaudit.TargetConsignment,
			Failure:    true,
			Metadata: map[string]any{
				"error": err.Error(),
			},
		})
		http.Error(w, "failed to create consignment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if consignment == nil {
		slog.Error("consignment is nil after successful creation")
		c.audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventConsignment,
			Action:     nswaudit.ActionCreate,
			TargetType: nswaudit.TargetConsignment,
			Failure:    true,
			Metadata: map[string]any{
				"error": "consignment is nil after successful creation",
			},
		})
		http.Error(w, "failed to create consignment: empty response", http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(consignment); err != nil {
		slog.Error("failed to encode response for consignment", "error", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetConsignments handles GET /api/v1/consignments
// Query params: role=trader | role=cha (defaults to trader).
func (c *Router) HandleGetConsignments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	role := r.URL.Query().Get("role")
	if role == "" {
		role = "trader"
	}
	offset, limit, err := pagination.ParsePaginationParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filter := Filter{
		Offset: offset,
		Limit:  limit,
	}

	// Optional Filters
	if stateStr := r.URL.Query().Get("state"); stateStr != "" {
		state := State(stateStr)
		filter.State = &state
	}
	if flowStr := r.URL.Query().Get("flow"); flowStr != "" {
		flow := Flow(flowStr)
		filter.Flow = &flow
	}

	// Role-based identity resolution.
	if role != "trader" && role != "cha" {
		http.Error(w, "query param role must be trader or cha", http.StatusBadRequest)
		return
	}

	userCompany, err := c.company.GetCompanyByOUHandle(ctx, authCtx.User.OUHandle)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) {
			http.Error(w, "company profile not found for user", http.StatusForbidden)
			return
		}
		slog.Error("failed to resolve user company", "ouHandle", authCtx.User.OUHandle, "error", err)
		http.Error(w, "failed to resolve user company", http.StatusInternalServerError)
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
		slog.Error("failed to retrieve consignments", "error", err)
		http.Error(w, "failed to retrieve consignments", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(consignments); err != nil {
		slog.Error("failed to encode response", "error", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetConsignmentByID handles GET /api/v1/consignments/{id}.
func (c *Router) HandleGetConsignmentByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	consignmentIDStr := r.PathValue("id")
	if consignmentIDStr == "" {
		http.Error(w, "consignment ID is required", http.StatusBadRequest)
		return
	}
	consignmentID := consignmentIDStr

	consignment, err := c.cs.GetConsignmentByID(r.Context(), consignmentID)
	if err != nil {
		if errors.Is(err, ErrConsignmentNotFound) {
			http.Error(w, "consignment not found", http.StatusNotFound)
			return
		}
		slog.Error("failed to retrieve consignment", "error", err)
		http.Error(w, "failed to retrieve consignment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(consignment); err != nil {
		slog.Error("failed to encode response", "error", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
