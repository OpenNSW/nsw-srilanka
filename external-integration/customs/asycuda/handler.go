package asycuda

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

const (
	errInvalidRequestPayload     = "invalid request payload"
	errUnknownEventType          = "unknown or unsupported event type"
	errWorkflowNotFound          = "workflow not found"
	errDeclarationNotFoundRetry  = "declaration not found, retry later"
	errDispatchNoteNotFound      = "dispatch note not found"
	errDispatchNoteNotFoundRetry = "dispatch note not found, retry later"
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
}

// HandleWebhook is the central entry point for POST /webhooks/slce.
// It inspects the eventType field in the incoming JSON payload and dispatches
// execution to the appropriate domain service handler using a switch statement.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.WarnContext(r.Context(), "slce: failed to read request body", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var env payloadEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode JSON envelope", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	eventType := strings.ToUpper(strings.TrimSpace(env.EventType))

	switch eventType {
	case "CUSDEC_INTEGRATED":
		h.handleCusdecIntegrationResult(w, r, body)

	case "PAYMENT_CONFIRMED":
		h.handleCusdecEvent(w, r, body, "PAYMENT_CONFIRMED")

	case "WARRANTING_COMPLETED":
		h.handleCusdecEvent(w, r, body, "WARRANTING_COMPLETED")

	case "EXPORT_RELEASED":
		h.handleCusdecEvent(w, r, body, "EXPORT_RELEASED")

	case "CDN_INTEGRATED":
		h.handleCDNIntegrationResult(w, r, body)

	case "CDN_ACKNOWLEDGED":
		h.handleCDNAcknowledgment(w, r, body)

	default:
		slog.WarnContext(r.Context(), "slce: unknown or unsupported event type", "event", eventType)
		httputil.Error(w, r, http.StatusBadRequest, errUnknownEventType)
	}
}

func (h *Handler) handleCusdecIntegrationResult(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cusdec.CusdecIntegrationResultRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CusDec integration result payload", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CusDec integration result validation failed", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := h.cusdecService.ProcessIntegrationResult(r.Context(), req); err != nil {
		if errors.Is(err, cusdec.ErrWorkflowNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "slce: workflow not found for CusDec integration result",
				"edge_id", req.EdgeID, "error", err)
			httputil.Error(w, r, http.StatusNotFound, errWorkflowNotFound)
			return
		}
		httputil.InternalServerError(w, r, "slce: failed to process CusDec integration result", err, "edge_id", req.EdgeID)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleCusdecEvent(w http.ResponseWriter, r *http.Request, body []byte, expectedEvent string) {
	var req cusdec.CusdecEventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CusDec event payload", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	// Ensure req.Event matches the target event expected by the domain service
	req.Event = expectedEvent

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CusDec event validation failed", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := h.cusdecService.ProcessEvent(r.Context(), req); err != nil {
		if errors.Is(err, cusdec.ErrCusdecNotFoundByRef) {
			slog.WarnContext(r.Context(), "slce: CusDec declaration not found for event (may be transient)",
				"cusdec_ref", req.Payload.CusdecRef, "error", err)
			httputil.Error(w, r, http.StatusServiceUnavailable, errDeclarationNotFoundRetry)
			return
		}
		if errors.Is(err, cusdec.ErrWorkflowNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "slce: workflow not found for CusDec event",
				"cusdec_ref", req.Payload.CusdecRef, "error", err)
			httputil.Error(w, r, http.StatusNotFound, errWorkflowNotFound)
			return
		}
		httputil.InternalServerError(w, r, "slce: failed to process CusDec event", err, "cusdec_ref", req.Payload.CusdecRef)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleCDNIntegrationResult(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cdn.CDNIntegrationResultRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CDN integration result payload", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CDN integration result validation failed", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := h.cdnService.ProcessIntegrationResult(r.Context(), req); err != nil {
		if errors.Is(err, cdn.ErrDispatchNoteNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "slce: dispatch note not found for integration result",
				"edge_id", req.EdgeID, "error", err)
			httputil.Error(w, r, http.StatusNotFound, errDispatchNoteNotFound)
			return
		}
		httputil.InternalServerError(w, r, "slce: failed to process CDN integration result", err, "edge_id", req.EdgeID)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleCDNAcknowledgment(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cdn.CDNAcknowledgmentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CDN acknowledgment payload", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CDN acknowledgment validation failed", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := h.cdnService.ProcessAcknowledgment(r.Context(), req); err != nil {
		if errors.Is(err, cdn.ErrDispatchNoteNotFoundByCDNRef) {
			slog.WarnContext(r.Context(), "slce: dispatch note not found for acknowledgment (may be transient)",
				"cdn_ref", req.Payload.CDNRef, "error", err)
			httputil.Error(w, r, http.StatusServiceUnavailable, errDispatchNoteNotFoundRetry)
			return
		}
		httputil.InternalServerError(w, r, "slce: failed to process CDN acknowledgment", err, "cdn_ref", req.Payload.CDNRef)
		return
	}

	w.WriteHeader(http.StatusOK)
}
