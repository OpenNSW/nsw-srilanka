package staticdata

import (
	"errors"
	"net/http"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
)

// cacheControlImmutable tells the browser/CDN it never needs to re-fetch a
// given (id, version) response, since that pair identifies immutable content.
const cacheControlImmutable = "public, max-age=31536000, immutable"

// Handler serves static JSON reference-data artifacts looked up by id and version.
type Handler struct {
	reg *artifact.Registry
}

// NewHandler creates a new static data HTTP handler.
func NewHandler(reg *artifact.Registry) *Handler {
	return &Handler{reg: reg}
}

// HandleGet handles GET /api/v1/static-data/{id}?version=.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	version := r.URL.Query().Get("version")
	if version == "" {
		httputil.Error(w, r, http.StatusBadRequest, "version query parameter is required")
		return
	}

	raw, err := Load(r.Context(), h.reg, id, version)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			httputil.Error(w, r, http.StatusNotFound, "static data not found")
			return
		}
		httputil.InternalServerError(w, r, "failed to load static data", err, "id", id, "version", version)
		return
	}

	w.Header().Set("Cache-Control", cacheControlImmutable)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw) //nolint:gosec // G705 false positive: raw is validated JSON (loadable.Parse), served as application/json, not HTML.
}
