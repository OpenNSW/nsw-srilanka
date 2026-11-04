package cha

import (
	"net/http"

	"github.com/OpenNSW/core/httputil"
)

// Handler exposes CHA profile endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a new CHA HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// HandleGetCHAs responds with every CHA. No route mounts it and no scope gates
// it: mounting it again needs a route in mountTNSW and a scope in
// internal/scopes, the IdP bootstrap and the portals' IDP_SCOPES.
func (h *Handler) HandleGetCHAs(w http.ResponseWriter, r *http.Request) {
	chas, err := h.svc.List(r.Context())
	if err != nil {
		httputil.InternalServerError(w, r, "failed to retrieve CHAs", err)
		return
	}

	httputil.JSON(w, http.StatusOK, chas)
}
