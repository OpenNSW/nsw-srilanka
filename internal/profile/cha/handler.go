package cha

import (
	"encoding/json"
	"net/http"
)

// Handler exposes CHA profile endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a new CHA HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// HandleGetCHAs handles GET /api/v1/chas
func (h *Handler) HandleGetCHAs(w http.ResponseWriter, r *http.Request) {
	chas, err := h.svc.List(r.Context())
	if err != nil {
		http.Error(w, "failed to retrieve CHAs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(chas); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
