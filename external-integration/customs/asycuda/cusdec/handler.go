package cusdec

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// HTTPHandler handles inbound HTTP webhook requests from ASYCUDA for Customs Declaration callbacks.
type HTTPHandler struct {
	service WebhookService
}

// NewHTTPHandler creates a new handler for CusDec webhooks.
func NewHTTPHandler(service WebhookService) *HTTPHandler {
	return &HTTPHandler{
		service: service,
	}
}

// HandleIntegrationResult handles POST /webhooks/asycuda/cusdec/result.
func (h *HTTPHandler) HandleIntegrationResult(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	defer func() { _ = r.Body.Close() }()

	var req CusdecIntegrationResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "asycuda: failed to decode CusDec integration result payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.validate(); err != nil {
		slog.WarnContext(r.Context(), "asycuda: CusDec integration result validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.ProcessIntegrationResult(r.Context(), req); err != nil {
		if errors.Is(err, ErrWorkflowNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "asycuda: workflow not found for CusDec integration result",
				"edge_id", req.EdgeID, "error", err)
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "asycuda: failed to process CusDec integration result",
			"edge_id", req.EdgeID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *HTTPHandler) handleEvent(w http.ResponseWriter, r *http.Request, expectedEvent string) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	defer func() { _ = r.Body.Close() }()

	var req CusdecEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "asycuda: failed to decode CusDec event payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.validate(); err != nil {
		slog.WarnContext(r.Context(), "asycuda: CusDec event validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Event != expectedEvent {
		slog.WarnContext(r.Context(), "asycuda: unexpected CusDec event type", "expected", expectedEvent, "got", req.Event)
		http.Error(w, "unexpected event type", http.StatusBadRequest)
		return
	}

	if err := h.service.ProcessEvent(r.Context(), req); err != nil {
		if errors.Is(err, ErrCusdecNotFoundByRef) {
			slog.WarnContext(r.Context(), "asycuda: CusDec declaration not found for event (may be transient)",
				"cusdec_ref", req.Payload.CusdecRef, "error", err)
			http.Error(w, "declaration not found, retry later", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, ErrWorkflowNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "asycuda: workflow not found for CusDec event",
				"cusdec_ref", req.Payload.CusdecRef, "error", err)
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "asycuda: failed to process CusDec event",
			"cusdec_ref", req.Payload.CusdecRef, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// HandlePayment handles POST /webhooks/asycuda/cusdec/payment.
func (h *HTTPHandler) HandlePayment(w http.ResponseWriter, r *http.Request) {
	h.handleEvent(w, r, "PAYMENT")
}

// HandleWarranting handles POST /webhooks/asycuda/cusdec/warranting.
func (h *HTTPHandler) HandleWarranting(w http.ResponseWriter, r *http.Request) {
	h.handleEvent(w, r, "WARRANTING")
}

// HandleRelease handles POST /webhooks/asycuda/cusdec/release.
func (h *HTTPHandler) HandleRelease(w http.ResponseWriter, r *http.Request) {
	h.handleEvent(w, r, "RELEASE")
}
