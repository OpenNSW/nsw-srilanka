package staticdata

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/pagination"
)

// cacheControlImmutable tells the caller's own browser it never needs to
// re-fetch a given (id, manifestVersion) response, since a published manifest
// never changes what it lists. "private" (not "public") because this endpoint
// is scope-protected — a shared/CDN cache must never serve one caller's
// response to another. Search responses are cached the same way: the query
// string is part of the URL, and the artifact behind it does not change.
const cacheControlImmutable = "private, max-age=31536000, immutable"

// Handler serves static JSON reference-data artifacts looked up by id, as an
// artifact manifest version lists them.
type Handler struct {
	artifacts *artifact.Set
}

// NewHandler creates a new static data HTTP handler.
func NewHandler(artifacts *artifact.Set) *Handler {
	return &Handler{artifacts: artifacts}
}

// parseManifestVersion reads the required manifestVersion query parameter, a
// positive integer.
func parseManifestVersion(r *http.Request) (int, error) {
	q := r.URL.Query()
	if !q.Has("manifestVersion") {
		return 0, errors.New("manifestVersion query parameter is required")
	}
	n, err := strconv.Atoi(q.Get("manifestVersion"))
	if err != nil || n <= 0 {
		return 0, errors.New("manifestVersion must be a positive integer")
	}
	return n, nil
}

// HandleGet handles GET /api/v1/static-data/{id}?manifestVersion=.
// Without q, offset, or limit it returns the raw artifact. With any of those
// it loads the artifact, applies parent when present, ranks string matches,
// and returns one page.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	manifestVersion, err := parseManifestVersion(r)
	if err != nil {
		httputil.Error(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if isSearch(r) {
		h.handleSearch(w, r, id, manifestVersion)
		return
	}

	raw, err := LoadAt(r.Context(), h.artifacts, id, manifestVersion)
	if err != nil {
		writeLoadError(w, r, err, id, manifestVersion)
		return
	}

	w.Header().Set("Cache-Control", cacheControlImmutable)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw) //nolint:gosec // G705 false positive: raw is validated JSON (loadable.Parse), served as application/json, not HTML.
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request, id string, manifestVersion int) {
	offset, limit, err := pagination.ParsePaginationParams(r)
	if err != nil {
		slog.WarnContext(r.Context(), "invalid pagination parameters", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, "invalid pagination parameters")
		return
	}
	finalOffset, finalLimit := pagination.ResolvePaginationParams(offset, limit)

	raw, err := LoadAt(r.Context(), h.artifacts, id, manifestVersion)
	if err != nil {
		writeLoadError(w, r, err, id, manifestVersion)
		return
	}

	result, err := Search(raw, strings.TrimSpace(r.URL.Query().Get("q")), r.URL.Query().Get("parent"), finalOffset, finalLimit)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to search static data", err, "id", id, "manifestVersion", manifestVersion)
		return
	}

	w.Header().Set("Cache-Control", cacheControlImmutable)
	httputil.JSON(w, http.StatusOK, result)
}

func isSearch(r *http.Request) bool {
	q := r.URL.Query()
	return q.Has("q") || q.Has("offset") || q.Has("limit")
}

func writeLoadError(w http.ResponseWriter, r *http.Request, err error, id string, manifestVersion int) {
	if errors.Is(err, artifact.ErrNotFound) || errors.Is(err, artifact.ErrUnknownManifestVersion) {
		httputil.Error(w, r, http.StatusNotFound, "static data not found")
		return
	}
	httputil.InternalServerError(w, r, "failed to load static data", err, "id", id, "manifestVersion", manifestVersion)
}
