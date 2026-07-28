package cha

import (
	"net/http"

	"github.com/OpenNSW/nsw-srilanka/internal/httputil"
)

// Handler exposes CHA profile endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a new CHA HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// HandleGetCHAs handles GET /api/v1/chas.
func (h *Handler) HandleGetCHAs(w http.ResponseWriter, r *http.Request) {
	chas, err := h.svc.List(r.Context())
	if err != nil {
		httputil.InternalServerError(w, r, "failed to retrieve CHAs", err)
		return
	}

	httputil.JSON(w, http.StatusOK, chas)
}
