package asycuda

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// HTTPHandler handles inbound HTTP webhook requests from ASYCUDA for Cargo
// Dispatch Note lifecycle callbacks.
//
// Source of truth: ASYCUDA API Specification v1.2.
//
// Security: These endpoints should be protected by OAuth 2.0 bearer-token
// validation in production. OAuth is currently disabled in the Customs test
// environment (CIG), so no auth middleware is wired here. When OAuth is
// re-enabled, add token verification middleware in front of these handlers.
type HTTPHandler struct {
	service       CDNWebhookService
	cusdecService CusdecWebhookService
}

// NewHTTPHandler creates a new handler wired to the given services.
func NewHTTPHandler(service CDNWebhookService, cusdecService CusdecWebhookService) *HTTPHandler {
	return &HTTPHandler{
		service:       service,
		cusdecService: cusdecService,
	}
}

// HandleIntegrationResult handles POST /webhooks/asycuda/cdn/result
//
// This is the §7.2 callback pushed by ASYCUDA when CDN integration succeeds or
// fails. The handler unmarshals and validates the payload, delegates to the
// service layer, and returns 202 Accepted so ASYCUDA does not retry.
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
		// Unknown dispatch note is a permanent error — stop retries.
		if errors.Is(err, ErrDispatchNoteNotFoundByEdgID) {
			slog.WarnContext(r.Context(), "asycuda: dispatch note not found for integration result",
				"edg_id", req.EdgID, "error", err)
			http.Error(w, "dispatch note not found", http.StatusNotFound)
			return
		}
		// Everything else is transient — 500 lets ASYCUDA retry.
		slog.ErrorContext(r.Context(), "asycuda: failed to process integration result",
			"edg_id", req.EdgID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// HandleAcknowledgment handles POST /webhooks/asycuda/cdn/ack
//
// This is the §7.3 callback pushed by ASYCUDA as a notification after the CDN
// has been acknowledged. The handler unmarshals and validates the payload,
// delegates to the service layer, and returns 202 Accepted.
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
		// If the note isn't found by cdnRef yet, it could be a transient race condition
		// (e.g. acknowledgment arrives before the integration result callback finishes processing).
		// Return 503 Service Unavailable to trigger an ASYCUDA retry.
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
