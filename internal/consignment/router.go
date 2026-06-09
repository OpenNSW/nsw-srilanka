package consignment

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/pagination"
	"github.com/OpenNSW/nsw/backend/internal/profile/cha"
	"github.com/OpenNSW/nsw/backend/internal/profile/company"
)

type Router struct {
	cs      *Service
	cha     cha.Service
	company company.Service
}

func NewRouter(cs *Service, chaService cha.Service, companyService company.Service) *Router {
	return &Router{cs: cs, cha: chaService, company: companyService}
}

// HandleCreateConsignment handles POST /api/v1/consignments
// Stage 1 (two-stage): body { flow, chaId } → creates shell (INITIALIZED)
func (c *Router) HandleCreateConsignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req CreateConsignmentDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	traderID := authCtx.User.ID
	// Stage 1: create shell only
	consignment, err := c.cs.CreateConsignmentShell(r.Context(), req.Flow, req.ChaCompanyID, traderID)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) {
			http.Error(w, "CHA company not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrCompanyNotCHA) {
			http.Error(w, "selected company is not a CHA company", http.StatusBadRequest)
			return
		}
		slog.Error("failed to create consignment shell", "error", err)
		http.Error(w, "failed to create consignment: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(consignment); err != nil {
		slog.Error("failed to encode response for consignment", "error", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleStartConsignment handles POST /api/v1/consignments/start
// Creates an export consignment and starts its workflow directly — no CHA company or HS code
// is collected up front; the workflow's own tasks collect those later. Response: DetailDTO.
func (c *Router) HandleStartConsignment(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "failed to create consignment: "+err.Error(), http.StatusInternalServerError)
		return
	}
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

// HandleInitializeConsignment handles PUT /api/v1/consignments/{id} (Stage 2: CHA selects Workflow Template).
// Body: InitializeConsignmentDTO { workflowTemplateId: string }. Response: DetailDTO.
func (c *Router) HandleInitializeConsignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	defer func() { _ = r.Body.Close() }()

	consignmentIDStr := r.PathValue("id")
	if consignmentIDStr == "" {
		http.Error(w, "consignment ID is required", http.StatusBadRequest)
		return
	}
	consignmentID := consignmentIDStr
	var req InitializeConsignmentDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.WorkflowTemplateID == "" {
		http.Error(w, "workflowTemplateId is required", http.StatusBadRequest)
		return
	}

	// Resolve the CHA picking up the consignment from the authenticated user's email.
	chaRecord, err := c.cha.GetByEmail(ctx, authCtx.User.Email)
	if err != nil {
		if errors.Is(err, cha.ErrCHANotFound) {
			http.Error(w, "CHA profile not found for user", http.StatusForbidden)
			return
		}
		slog.Error("failed to resolve CHA profile", "email", authCtx.User.Email, "error", err)
		http.Error(w, "failed to resolve CHA profile", http.StatusInternalServerError)
		return
	}

	consignment, err := c.cs.InitializeConsignmentByID(r.Context(), consignmentID, req.WorkflowTemplateID, chaRecord.ID)
	if err != nil {
		if errors.Is(err, ErrConsignmentNotFound) {
			http.Error(w, "consignment not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrCHACompanyMismatch) {
			http.Error(w, "CHA does not belong to the consignment's CHA company", http.StatusForbidden)
			return
		}
		slog.Error("failed to initialize consignment", "error", err)
		http.Error(w, "failed to initialize consignment: "+err.Error(), http.StatusInternalServerError)
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

// HandleGetConsignmentByID handles GET /api/v1/consignments/{id}
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
