package asycuda

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

// Handler handles central inbound HTTP webhook requests from SLCE / ASYCUDA
// routed to POST /webhooks/slce.
type Handler struct {
	cusdecService cusdec.WebhookService
	cdnService    cdn.CDNWebhookService
}

// NewHandler creates a new central SLCE webhook handler.
func NewHandler(cusdecService cusdec.WebhookService, cdnService cdn.CDNWebhookService) *Handler {
	return &Handler{
		cusdecService: cusdecService,
		cdnService:    cdnService,
	}
}

type payloadEnvelope struct {
	EventType string `json:"eventType"`
	Event     string `json:"event"`
}

// HandleWebhook is the central entry point for POST /webhooks/slce.
// It inspects the eventType (or event) field in the incoming JSON payload and dispatches
// execution to the appropriate domain service handler using a switch statement.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.WarnContext(r.Context(), "slce: failed to read request body", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var env payloadEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode JSON envelope", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	eventType := strings.ToUpper(strings.TrimSpace(env.EventType))
	if eventType == "" {
		eventType = strings.ToUpper(strings.TrimSpace(env.Event))
	}

	switch eventType {
	case "INTEGRATION_RESULT":
		h.handleCusdecIntegrationResult(w, r, body)

	case "PAYMENT", "PAYMENT_NOTIFICATION":
		h.handleCusdecEvent(w, r, body, "PAYMENT")

	case "WARRANTING", "WARRANTING_NOTIFICATION":
		h.handleCusdecEvent(w, r, body, "WARRANTING")

	case "RELEASE", "RELEASE_NOTIFICATION":
		h.handleCusdecEvent(w, r, body, "RELEASE")

	case "CDN_INTEGRATION_RESULT":
		h.handleCDNIntegrationResult(w, r, body)

	case "ACKNOWLEDGMENT", "CDN_ACKNOWLEDGMENT":
		h.handleCDNAcknowledgment(w, r, body)

	default:
		slog.WarnContext(r.Context(), "slce: unknown or unsupported event type", "event", eventType)
		http.Error(w, "unknown or unsupported event type", http.StatusBadRequest)
	}
}

func (h *Handler) handleCusdecIntegrationResult(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cusdec.CusdecIntegrationResultRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CusDec integration result payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CusDec integration result validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.cusdecService.ProcessIntegrationResult(r.Context(), req); err != nil {
		if errors.Is(err, cusdec.ErrWorkflowNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "slce: workflow not found for CusDec integration result",
				"edge_id", req.EdgeID, "error", err)
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "slce: failed to process CusDec integration result",
			"edge_id", req.EdgeID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) handleCusdecEvent(w http.ResponseWriter, r *http.Request, body []byte, expectedEvent string) {
	var req cusdec.CusdecEventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CusDec event payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	// Ensure req.Event matches the target event expected by the domain service
	req.Event = expectedEvent

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CusDec event validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.cusdecService.ProcessEvent(r.Context(), req); err != nil {
		if errors.Is(err, cusdec.ErrCusdecNotFoundByRef) {
			slog.WarnContext(r.Context(), "slce: CusDec declaration not found for event (may be transient)",
				"cusdec_ref", req.Payload.CusdecRef, "error", err)
			http.Error(w, "declaration not found, retry later", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, cusdec.ErrWorkflowNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "slce: workflow not found for CusDec event",
				"cusdec_ref", req.Payload.CusdecRef, "error", err)
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "slce: failed to process CusDec event",
			"cusdec_ref", req.Payload.CusdecRef, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) handleCDNIntegrationResult(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cdn.CDNIntegrationResultRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CDN integration result payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CDN integration result validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.cdnService.ProcessIntegrationResult(r.Context(), req); err != nil {
		if errors.Is(err, cdn.ErrDispatchNoteNotFoundByEdgID) {
			slog.WarnContext(r.Context(), "slce: dispatch note not found for integration result",
				"edg_id", req.EdgID, "error", err)
			http.Error(w, "dispatch note not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "slce: failed to process CDN integration result",
			"edg_id", req.EdgID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) handleCDNAcknowledgment(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cdn.CDNAcknowledgmentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CDN acknowledgment payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CDN acknowledgment validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.cdnService.ProcessAcknowledgment(r.Context(), req); err != nil {
		if errors.Is(err, cdn.ErrDispatchNoteNotFoundByCDNRef) {
			slog.WarnContext(r.Context(), "slce: dispatch note not found for acknowledgment (may be transient)",
				"cdn_ref", req.Payload.CDNRef, "error", err)
			http.Error(w, "dispatch note not found, retry later", http.StatusServiceUnavailable)
			return
		}
		slog.ErrorContext(r.Context(), "slce: failed to process CDN acknowledgment",
			"cdn_ref", req.Payload.CDNRef, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
