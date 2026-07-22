package cdn

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// HTTPHandler handles inbound HTTP webhook requests from ASYCUDA for Cargo
// Dispatch Note lifecycle callbacks.
type HTTPHandler struct {
	service CDNWebhookService
}

// NewHTTPHandler creates a new handler wired to the given CDN webhook service.
func NewHTTPHandler(service CDNWebhookService) *HTTPHandler {
	return &HTTPHandler{
		service: service,
	}
}

// HandleIntegrationResult handles POST /webhooks/asycuda/cdn/result.
func (h *HTTPHandler) HandleIntegrationResult(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	defer func() { _ = r.Body.Close() }()

	var req CDNIntegrationResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "asycuda: failed to decode integration result payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.validate(); err != nil {
		slog.WarnContext(r.Context(), "asycuda: integration result validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.ProcessIntegrationResult(r.Context(), req); err != nil {
		if errors.Is(err, ErrDispatchNoteNotFoundByEdgID) {
			slog.WarnContext(r.Context(), "asycuda: dispatch note not found for integration result",
				"edg_id", req.EdgID, "error", err)
			http.Error(w, "dispatch note not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "asycuda: failed to process integration result",
			"edg_id", req.EdgID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// HandleAcknowledgment handles POST /webhooks/asycuda/cdn/ack.
func (h *HTTPHandler) HandleAcknowledgment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	defer func() { _ = r.Body.Close() }()

	var req CDNAcknowledgmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "asycuda: failed to decode acknowledgment payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.validate(); err != nil {
		slog.WarnContext(r.Context(), "asycuda: acknowledgment validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.ProcessAcknowledgment(r.Context(), req); err != nil {
		if errors.Is(err, ErrDispatchNoteNotFoundByCDNRef) {
			slog.WarnContext(r.Context(), "asycuda: dispatch note not found for acknowledgment (may be transient)",
				"cdn_ref", req.Payload.CDNRef, "error", err)
			http.Error(w, "dispatch note not found, retry later", http.StatusServiceUnavailable)
			return
		}
		slog.ErrorContext(r.Context(), "asycuda: failed to process acknowledgment",
			"cdn_ref", req.Payload.CDNRef, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
