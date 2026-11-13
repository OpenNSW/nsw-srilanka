package asycuda

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// maxAuditIDLen bounds callback-controlled correlation metadata in bytes. Oversized
// values are omitted, not truncated into a different identifier; lookups use the original.
const maxAuditIDLen = 128

// WithAuditor enables callback outcome audits: one record for each delivery, and,
// for a callback held for retry, one more for how its retries ended. Repositories
// resolve document IDs after processing; a missing document or lookup error never
// changes the HTTP result. Configure it before serving requests, like the other
// handler dependencies.
func (h *Handler) WithAuditor(a nswaudit.Auditor, declarations cusdec.DeclarationRepository, notes cdn.DispatchNoteRepository) *Handler {
	h.auditor, h.declarations, h.dispatchNotes = a, declarations, notes
	return h
}

// callbackIdentity deliberately decodes only correlation fields. Payloads can contain
// full declarations, duties, and error descriptions, which must not enter audit logs.
type callbackIdentity struct {
	EventType string `json:"eventType"`
	Payload   struct {
		EdgeID     string             `json:"edgeId"`
		CusdecRef  document.Reference `json:"cusDecRef"`
		CDNRef     document.Reference `json:"cdnRef"`
		Integrated *bool              `json:"integrated"`
	} `json:"payload"`
}

// auditCallback emits one bounded delivery outcome without copying arbitrary payload
// fields. held marks a callback accepted for retry: its 202 says it was received,
// not applied, and a second record follows once its retries end (auditRetryOutcome).
func (h *Handler) auditCallback(ctx context.Context, body []byte, status int, held bool) {
	// No response was written if processing panicked. Never report that as success.
	if status == 0 {
		status = http.StatusInternalServerError
	}
	e := h.callbackEvent(ctx, body, status >= 400)
	e.Metadata["httpStatus"] = status
	if held {
		e.Metadata["heldForRetry"] = true
	}
	h.auditor.Audit(ctx, e)
}

// auditRetryOutcome records how a callback held for retry ended after attempts
// tries: applied (err nil), refused as one no retry can fix, or given up when the
// retry window closed. The record shares the delivery's actor and trace.
func (h *Handler) auditRetryOutcome(ctx context.Context, cb Callback, attempts int, err error) {
	// The delivery was audited only for a known caller; its retries carry the same context.
	if _, known := authn.FromContext(ctx); !known || h.auditor == nil {
		return
	}
	e := h.callbackEvent(ctx, cb.Body, err != nil)
	e.Metadata["retryAttempts"] = attempts
	switch {
	case err == nil:
		e.Metadata["retryOutcome"] = "applied"
	case isPermanent(err):
		e.Metadata["retryOutcome"] = "refused"
		e.Metadata["error_code"] = "callback_retry_refused"
	default:
		e.Metadata["retryOutcome"] = "gave_up"
		e.Metadata["error_code"] = "callback_retry_exhausted"
	}
	h.auditor.Audit(ctx, e)
}

// callbackEvent is the audit event for the callback in body: its allowlisted event
// type, its bounded identifiers, and the local document it resolves to.
func (h *Handler) callbackEvent(ctx context.Context, body []byte, failure bool) nswaudit.Event {
	e := nswaudit.Event{
		EventType: nswaudit.EventCustoms, Action: nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetCustomsCallback, Failure: failure,
		Metadata: map[string]any{"source": "slce"},
	}
	var identity callbackIdentity
	if err := json.Unmarshal(body, &identity); err != nil {
		e.Metadata["error_code"] = "invalid_callback_payload"
		return e
	}
	identity.EventType = strings.ToUpper(strings.TrimSpace(identity.EventType))
	e.Metadata["eventType"] = "UNKNOWN"
	if e.Failure {
		e.Metadata["error_code"] = "callback_rejected"
	}
	if identity.Payload.EdgeID != "" && len(identity.Payload.EdgeID) <= maxAuditIDLen {
		e.Metadata["edgeId"] = identity.Payload.EdgeID
	}
	if identity.Payload.Integrated != nil {
		e.Metadata["integrated"] = *identity.Payload.Integrated
	}
	switch identity.EventType {
	case "CUSDEC_INTEGRATED", "PAYMENT_CONFIRMED", "WARRANTING_COMPLETED", "EXPORT_RELEASED", cusdec.EventCusdecAmended, cusdec.EventCusdecCancelled:
		e.Metadata["eventType"] = identity.EventType
		e.TargetType = nswaudit.TargetDeclaration
		if ref := auditDocumentReference(identity.Payload.CusdecRef); ref != "" {
			e.Metadata["documentRef"] = ref
		}
		h.resolveDeclarationTarget(ctx, identity, &e)
	case "CDN_INTEGRATED", "CDN_ACKNOWLEDGED", cdn.EventCDNAmended, cdn.EventCDNCancelled:
		e.Metadata["eventType"] = identity.EventType
		e.TargetType = nswaudit.TargetDispatchNote
		if ref := auditDocumentReference(identity.Payload.CDNRef); ref != "" {
			e.Metadata["documentRef"] = ref
		}
		h.resolveDispatchNoteTarget(ctx, identity, &e)
	}
	if e.TargetType != nswaudit.TargetCustomsCallback && e.TargetID == "" {
		e.Metadata["targetResolutionError"] = "callback document could not be resolved"
	}
	return e
}

// auditDocumentReference returns only complete references that fit the metadata bound.
func auditDocumentReference(ref document.Reference) string {
	if !ref.IsValid() {
		return ""
	}
	value := ref.String()
	if len(value) > maxAuditIDLen {
		return ""
	}
	return value
}

// resolveDeclarationTarget uses the original callback identifiers to find a local declaration.
func (h *Handler) resolveDeclarationTarget(ctx context.Context, id callbackIdentity, e *nswaudit.Event) {
	if h.declarations == nil {
		return
	}
	var decl *cusdec.CusdecDeclaration
	var err error
	if id.EventType == "CUSDEC_INTEGRATED" && id.Payload.EdgeID != "" {
		decl, err = h.declarations.GetByEdgeID(ctx, id.Payload.EdgeID)
	} else if id.Payload.CusdecRef.IsValid() {
		decl, err = h.declarations.GetByCusdecRef(ctx, id.Payload.CusdecRef)
	}
	if err != nil {
		slog.WarnContext(ctx, "slce: audit declaration lookup failed", "error", err)
		return
	}
	if decl != nil {
		e.TargetID = decl.ID
		e.Metadata["caseId"] = decl.CaseID
	}
}

// resolveDispatchNoteTarget uses the original callback identifiers to find a local dispatch note.
func (h *Handler) resolveDispatchNoteTarget(ctx context.Context, id callbackIdentity, e *nswaudit.Event) {
	if h.dispatchNotes == nil {
		return
	}
	var note *cdn.DispatchNote
	var err error
	if id.EventType == "CDN_INTEGRATED" && id.Payload.EdgeID != "" {
		note, err = h.dispatchNotes.GetByEdgeID(ctx, id.Payload.EdgeID)
	} else if id.Payload.CDNRef.IsValid() {
		note, err = h.dispatchNotes.GetByCDNRef(ctx, id.Payload.CDNRef)
	}
	if err != nil {
		slog.WarnContext(ctx, "slce: audit dispatch note lookup failed", "error", err)
		return
	}
	if note != nil {
		e.TargetID = note.ID
		e.Metadata["caseId"] = note.CaseID
	}
}

// callbackResponse records the status without buffering or changing the response.
type callbackResponse struct {
	http.ResponseWriter
	status int
	// held is set when the callback was accepted to be retried in the background
	// instead of applied (see holdIfEarly).
	held bool
}

// WriteHeader captures the first response status and preserves duplicate-header behavior.
func (w *callbackResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write records an implicit 200 before forwarding the response body unchanged.
func (w *callbackResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap preserves ResponseController access to the underlying writer.
func (w *callbackResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
