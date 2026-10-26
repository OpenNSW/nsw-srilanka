package asycuda

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/edgetrace"
)

const (
	errInvalidRequestPayload     = "invalid request payload"
	errUnknownEventType          = "unknown or unsupported event type"
	errWorkflowNotFound          = "workflow not found"
	errCaseNotRecorded           = "declaration has no case recorded"
	errDeclarationNotFoundRetry  = "declaration not found, retry later"
	errDispatchNoteNotFound      = "dispatch note not found"
	errDispatchNoteNotFoundRetry = "dispatch note not found, retry later"
	errTaskNotParkedRetry        = "task not waiting yet, retry later"
	errNoteCaseNotRecorded       = "dispatch note has no case recorded"
)

// Handler handles central inbound HTTP webhook requests from SLCE / ASYCUDA
// routed to POST /webhooks/slce.
type Handler struct {
	cusdecService cusdec.WebhookService
	cdnService    cdn.CDNWebhookService

	// DebugForward is TEMPORARY debugging code: see debug_forward.go, which says
	// how to remove it. When set, every callback is also sent to the
	// slce-debug-forward service.
	DebugForward *remote.Manager
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

	edgetrace.Webhook(r.Context(), r.URL.Path, body)

	// TEMPORARY debugging code: see debug_forward.go.
	h.forwardDebugCopy(r.Context(), body)

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

	case cusdec.EventCusdecAmended:
		h.handleCusdecAmendment(w, r, body)

	case "CDN_INTEGRATED":
		h.handleCDNIntegrationResult(w, r, body)

	case "CDN_ACKNOWLEDGED":
		h.handleCDNAcknowledgment(w, r, body)

	case cdn.EventCDNAmended:
		h.handleCDNAmendment(w, r, body)

	case cdn.EventCDNCancelled:
		h.handleCDNCancellation(w, r, body)

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
		// A repeat delivery is acknowledged rather than re-run. 202 tells SLC
		// Edge the callback landed — ending the §2 retry schedule — while
		// staying distinguishable from the 200 that means work was done.
		if errors.Is(err, cusdec.ErrDuplicateIntegrationResult) {
			slog.InfoContext(r.Context(), "slce: duplicate CusDec integration result acknowledged",
				"edge_id", req.EdgeID)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if errors.Is(err, cusdec.ErrDeclarationNotRecorded) {
			slog.WarnContext(r.Context(), "slce: CusDec integration result arrived before its submission was recorded; retry",
				"edge_id", req.EdgeID)
			httputil.Error(w, r, http.StatusServiceUnavailable, errDeclarationNotFoundRetry)
			return
		}
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
		// An event already applied to this declaration is acknowledged with a
		// plain 200 and nothing else: the delivery succeeded, there was simply
		// no work left to do.
		if errors.Is(err, cusdec.ErrDuplicateEvent) {
			slog.InfoContext(r.Context(), "slce: duplicate CusDec event acknowledged",
				"cusdec_ref", req.Payload.CusdecRef, "event", req.Event)
			w.WriteHeader(http.StatusOK)
			return
		}
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

// handleCusdecAmendment opens the amendment flow (§6.5.4) on the amended
// declaration's case. A redelivery names the same flow, which is not opened
// twice, so it is answered like the first delivery.
func (h *Handler) handleCusdecAmendment(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cusdec.CusdecAmendedRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CusDec amendment payload", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}
	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CusDec amendment validation failed", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := h.cusdecService.ProcessAmendment(r.Context(), req); err != nil {
		ref := req.Payload.CusdecRef
		switch {
		case errors.Is(err, cusdec.ErrCusdecNotFoundByRef):
			// As for the other §6.5 events: the integration result that records
			// the reference may still be in flight, so SLC Edge is asked to retry.
			slog.WarnContext(r.Context(), "slce: CusDec declaration not found for amendment (may be transient)",
				"cusdec_ref", ref, "error", err)
			httputil.Error(w, r, http.StatusServiceUnavailable, errDeclarationNotFoundRetry)
		case errors.Is(err, cusdec.ErrCaseNotRecorded):
			slog.WarnContext(r.Context(), "slce: amended CusDec declaration has no case", "cusdec_ref", ref, "error", err)
			httputil.Error(w, r, http.StatusNotFound, errCaseNotRecorded)
		default:
			httputil.InternalServerError(w, r, "slce: failed to process CusDec amendment", err, "cusdec_ref", ref)
		}
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
		if errors.Is(err, cdn.ErrDispatchNoteNotRecorded) {
			slog.WarnContext(r.Context(), "slce: CDN integration result arrived before its submission was recorded; retry",
				"edge_id", req.Payload.EdgeID)
			httputil.Error(w, r, http.StatusServiceUnavailable, errDispatchNoteNotFoundRetry)
			return
		}
		if errors.Is(err, cdn.ErrTaskNotParked) {
			slog.WarnContext(r.Context(), "slce: CDN integration result arrived before its task was waiting; retry",
				"edge_id", req.Payload.EdgeID)
			httputil.Error(w, r, http.StatusServiceUnavailable, errTaskNotParkedRetry)
			return
		}
		if errors.Is(err, cdn.ErrDispatchNoteNotFoundByEdgeID) {
			slog.WarnContext(r.Context(), "slce: dispatch note not found for integration result",
				"edge_id", req.Payload.EdgeID, "error", err)
			httputil.Error(w, r, http.StatusNotFound, errDispatchNoteNotFound)
			return
		}
		httputil.InternalServerError(w, r, "slce: failed to process CDN integration result", err, "edge_id", req.Payload.EdgeID)
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
		if errors.Is(err, cdn.ErrTaskNotParked) {
			slog.WarnContext(r.Context(), "slce: CDN acknowledgment arrived before its task was waiting; retry",
				"cdn_ref", req.Payload.CDNRef)
			httputil.Error(w, r, http.StatusServiceUnavailable, errTaskNotParkedRetry)
			return
		}
		httputil.InternalServerError(w, r, "slce: failed to process CDN acknowledgment", err, "cdn_ref", req.Payload.CDNRef)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleCDNAmendment opens the amendment flow (§7.6) on the amended dispatch
// note's case. A redelivery names the same flow, which is not opened twice, so
// it is answered like the first delivery.
func (h *Handler) handleCDNAmendment(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cdn.CDNAmendedRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CDN amendment payload", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}
	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CDN amendment validation failed", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := h.cdnService.ProcessAmendment(r.Context(), req); err != nil {
		ref := req.Payload.CDNRef
		switch {
		case errors.Is(err, cdn.ErrDispatchNoteNotFoundByCDNRef):
			// As for the acknowledgment: the integration result that records the
			// reference may still be in flight, so SLC Edge is asked to retry.
			slog.WarnContext(r.Context(), "slce: dispatch note not found for amendment (may be transient)",
				"cdn_ref", ref, "error", err)
			httputil.Error(w, r, http.StatusServiceUnavailable, errDispatchNoteNotFoundRetry)
		case errors.Is(err, cdn.ErrCaseNotRecorded):
			slog.WarnContext(r.Context(), "slce: amended dispatch note has no case", "cdn_ref", ref, "error", err)
			httputil.Error(w, r, http.StatusNotFound, errNoteCaseNotRecorded)
		default:
			httputil.InternalServerError(w, r, "slce: failed to process CDN amendment", err, "cdn_ref", ref)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleCDNCancellation ends the cancelled dispatch note's branch (§7.7). A
// repeat delivery is answered like the first.
func (h *Handler) handleCDNCancellation(w http.ResponseWriter, r *http.Request, body []byte) {
	var req cdn.CDNCancelledRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.WarnContext(r.Context(), "slce: failed to decode CDN cancellation payload", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}
	if err := req.Validate(); err != nil {
		slog.WarnContext(r.Context(), "slce: CDN cancellation validation failed", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	if err := h.cdnService.ProcessCancellation(r.Context(), req); err != nil {
		ref := req.Payload.CDNRef
		if errors.Is(err, cdn.ErrDispatchNoteNotFoundByCDNRef) {
			// As for the acknowledgment: the integration result that records the
			// reference may still be in flight, so SLC Edge is asked to retry.
			slog.WarnContext(r.Context(), "slce: dispatch note not found for cancellation (may be transient)",
				"cdn_ref", ref, "error", err)
			httputil.Error(w, r, http.StatusServiceUnavailable, errDispatchNoteNotFoundRetry)
			return
		}
		if errors.Is(err, cdn.ErrTaskNotParked) {
			slog.WarnContext(r.Context(), "slce: CDN cancellation arrived before its task was waiting; retry",
				"cdn_ref", ref)
			httputil.Error(w, r, http.StatusServiceUnavailable, errTaskNotParkedRetry)
			return
		}
		httputil.InternalServerError(w, r, "slce: failed to process CDN cancellation", err, "cdn_ref", ref)
		return
	}

	w.WriteHeader(http.StatusOK)
}
