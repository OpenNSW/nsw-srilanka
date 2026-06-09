package company

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/OpenNSW/core/pagination"
)

// Handler exposes company profile endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a new company HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// HandleGetCompanies handles GET /api/v1/companies.
// Optional query params: has_cha (true|false), name (substring, case-insensitive), offset, limit.
func (h *Handler) HandleGetCompanies(w http.ResponseWriter, r *http.Request) {
	filter := ListFilter{}
	if v := r.URL.Query().Get("has_cha"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			http.Error(w, "invalid has_cha (expected true or false)", http.StatusBadRequest)
			return
		}
		filter.HasCHA = &parsed
	}
	if name := r.URL.Query().Get("name"); name != "" {
		filter.Name = &name
	}

	offset, limit, err := pagination.ParsePaginationParams(r)
	if err != nil {
		http.Error(w, "invalid pagination parameters", http.StatusBadRequest)
		slog.Error("invalid pagination parameters", "error", err)
		return
	}
	filter.Offset = offset
	filter.Limit = limit

	result, err := h.svc.ListCompanies(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to retrieve companies", http.StatusInternalServerError)
		slog.Error("failed to retrieve companies", "error", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("failed to encode company response", "error", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
