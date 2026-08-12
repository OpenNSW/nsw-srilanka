package storage

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/storage"
)

// SecureHTTPHandler wraps the core storage HTTPHandler with metadata-backed
// access validation and upload tracking. Core does not yet expose hook methods
// on its handler, so we enforce them here at the application layer.
type SecureHTTPHandler struct {
	inner    *storage.HTTPHandler
	metadata *ObjectMetadataService
}

func NewSecureHTTPHandler(inner *storage.HTTPHandler, metadata *ObjectMetadataService) *SecureHTTPHandler {
	return &SecureHTTPHandler{inner: inner, metadata: metadata}
}

func (h *SecureHTTPHandler) Upload(w http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()
	h.inner.Upload(rec, r)

	for k, vals := range rec.Header() {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.Code)

	if rec.Code == http.StatusOK && h.metadata != nil {
		var metadata storage.FileMetadata
		if err := json.Unmarshal(rec.Body.Bytes(), &metadata); err != nil {
			slog.ErrorContext(r.Context(), "failed to decode upload metadata for OnUpload hook", "error", err)
		} else if err := h.metadata.OnUpload(r.Context(), &metadata, authn.GetAuthContext(r.Context())); err != nil {
			slog.ErrorContext(r.Context(), "OnUpload hook failed", "key", metadata.Key, "error", err)
		}
	}

	if _, err := w.Write(rec.Body.Bytes()); err != nil {
		slog.ErrorContext(r.Context(), "failed to write upload response", "error", err)
	}
}

func (h *SecureHTTPHandler) Download(w http.ResponseWriter, r *http.Request) {
	if h.metadata != nil {
		key := r.PathValue("key")
		allowed, err := h.metadata.ValidateAccess(r.Context(), key, authn.GetAuthContext(r.Context()))
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to validate storage access", "key", key, "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to validate access")
			return
		}
		if !allowed {
			slog.WarnContext(r.Context(), "storage access denied", "key", key)
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	h.inner.Download(w, r)
}

func (h *SecureHTTPHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.inner.Delete(w, r)
}

func (h *SecureHTTPHandler) UploadContentLocal(w http.ResponseWriter, r *http.Request) {
	h.inner.UploadContentLocal(w, r)
}

func (h *SecureHTTPHandler) DownloadContent(w http.ResponseWriter, r *http.Request) {
	h.inner.DownloadContent(w, r)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
