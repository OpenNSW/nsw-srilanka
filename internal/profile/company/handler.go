package company

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/OpenNSW/core/httputil"
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
			httputil.Error(w, r, http.StatusBadRequest, "invalid has_cha (expected true or false)")
			return
		}
		filter.HasCHA = &parsed
	}
	if name := r.URL.Query().Get("name"); name != "" {
		filter.Name = &name
	}

	offset, limit, err := pagination.ParsePaginationParams(r)
	if err != nil {
		slog.WarnContext(r.Context(), "invalid pagination parameters", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, "invalid pagination parameters")
		return
	}
	filter.Offset = offset
	filter.Limit = limit

	result, err := h.svc.ListCompanies(r.Context(), filter)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to retrieve companies", err)
		return
	}

	httputil.JSON(w, http.StatusOK, result)
}
