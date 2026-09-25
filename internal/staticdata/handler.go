package staticdata

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/pagination"
)

// cacheControlImmutable tells the caller's own browser it never needs to
// re-fetch a given (id, version) response, since that pair identifies
// immutable content. "private" (not "public") because this endpoint is
// scope-protected — a shared/CDN cache must never serve one caller's response
// to another. Search responses are cached the same way: the query string is
// part of the URL, and the artifact behind (id, version) does not change.
const cacheControlImmutable = "private, max-age=31536000, immutable"

// Handler serves static JSON reference-data artifacts looked up by id and version.
type Handler struct {
	reg *artifact.Registry
}

// NewHandler creates a new static data HTTP handler.
func NewHandler(reg *artifact.Registry) *Handler {
	return &Handler{reg: reg}
}

// HandleGet handles GET /api/v1/static-data/{id}?version=.
// Without q, offset, or limit it returns the raw artifact. With any of those
// it loads the artifact, applies parent when present, ranks string matches,
// and returns one page.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	version := r.URL.Query().Get("version")
	if version == "" {
		httputil.Error(w, r, http.StatusBadRequest, "version query parameter is required")
		return
	}

	if isSearch(r) {
		h.handleSearch(w, r, id, version)
		return
	}

	raw, err := Load(r.Context(), h.reg, id, version)
	if err != nil {
		writeLoadError(w, r, err, id, version)
		return
	}

	w.Header().Set("Cache-Control", cacheControlImmutable)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw) //nolint:gosec // G705 false positive: raw is validated JSON (loadable.Parse), served as application/json, not HTML.
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request, id, version string) {
	offset, limit, err := pagination.ParsePaginationParams(r)
	if err != nil {
		slog.WarnContext(r.Context(), "invalid pagination parameters", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, "invalid pagination parameters")
		return
	}
	finalOffset, finalLimit := pagination.ResolvePaginationParams(offset, limit)

	raw, err := Load(r.Context(), h.reg, id, version)
	if err != nil {
		writeLoadError(w, r, err, id, version)
		return
	}

	result, err := Search(raw, strings.TrimSpace(r.URL.Query().Get("q")), r.URL.Query().Get("parent"), finalOffset, finalLimit)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to search static data", err, "id", id, "version", version)
		return
	}

	w.Header().Set("Cache-Control", cacheControlImmutable)
	httputil.JSON(w, http.StatusOK, result)
}

func isSearch(r *http.Request) bool {
	q := r.URL.Query()
	return q.Has("q") || q.Has("offset") || q.Has("limit")
}

func writeLoadError(w http.ResponseWriter, r *http.Request, err error, id, version string) {
	if errors.Is(err, artifact.ErrNotFound) {
		httputil.Error(w, r, http.StatusNotFound, "static data not found")
		return
	}
	httputil.InternalServerError(w, r, "failed to load static data", err, "id", id, "version", version)
}
