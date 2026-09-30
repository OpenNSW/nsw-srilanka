package storage

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// maxUploadRequestBytes caps the upload request body: it carries only the
// file's metadata, never its content.
const maxUploadRequestBytes = 1 << 20

// ProxyHandler serves the storage API routes in proxy mode, with the same
// request and response shapes as core/storage's HTTPHandler. File-level
// rules — allowed types, the size limit, the key format — are the owning
// service's: it enforces them, and its rejections are relayed to the client.
//
// Authentication is the route middleware's job; the handler assumes the
// caller has already been authenticated and authorized.
type ProxyHandler struct {
	svc *ProxyService
}

// NewProxyHandler creates a ProxyHandler over svc.
func NewProxyHandler(svc *ProxyService) *ProxyHandler {
	return &ProxyHandler{svc: svc}
}

// Upload reserves a key with the owning service and returns the file metadata,
// including the URL the client uploads the file to.
func (h *ProxyHandler) Upload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		MimeType string `json:"mime_type"`
		Size     int64  `json:"size"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Filename == "" {
		writeJSONError(w, http.StatusBadRequest, "filename is required")
		return
	}
	if req.MimeType == "" {
		writeJSONError(w, http.StatusBadRequest, "mime_type is required")
		return
	}
	if req.Size <= 0 {
		writeJSONError(w, http.StatusBadRequest, "size must be greater than 0")
		return
	}

	meta, err := h.svc.Upload(r.Context(), req.Filename, req.Size, req.MimeType)
	if err != nil {
		writeProxyError(w, r, err, "failed to prepare upload")
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

// Download returns the owning service's download URL for the key and when it
// expires.
func (h *ProxyHandler) Download(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "key is required")
		return
	}

	downloadURL, expiresAt, err := h.svc.DownloadURL(r.Context(), key)
	if err != nil {
		writeProxyError(w, r, err, "failed to generate access")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"download_url": downloadURL,
		"expires_at":   expiresAt,
	})
}

// Delete removes the file from the owning service.
func (h *ProxyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "key is required")
		return
	}

	if err := h.svc.Delete(r.Context(), key); err != nil {
		writeProxyError(w, r, err, "failed to delete file")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeProxyError relays a rejection of the request by the owning service
// with its status and message; anything else — the owning service being
// unreachable, failing, or refusing this service's credentials — is a
// gateway failure the client cannot fix.
func writeProxyError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	var rejection *RemoteRejection
	if errors.As(err, &rejection) {
		message := rejection.Message
		if message == "" {
			message = fallback
		}
		writeJSONError(w, rejection.StatusCode, message)
		return
	}
	slog.ErrorContext(r.Context(), "storage proxy request failed", "error", err)
	writeJSONError(w, http.StatusBadGateway, fallback)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode storage response", "error", err)
	}
}
