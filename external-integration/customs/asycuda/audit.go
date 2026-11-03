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
)

// maxAuditIDLen bounds callback-controlled correlation metadata in bytes. Oversized
// values are omitted, not truncated into a different identifier; lookups use the original.
const maxAuditIDLen = 128

// WithAuditor enables callback outcome audits. Repositories resolve document IDs
// after processing; a missing document or lookup error never changes the HTTP result.
// Configure it before serving requests, like the other handler dependencies.
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

// auditCallback emits one bounded delivery outcome without copying arbitrary payload fields.
func (h *Handler) auditCallback(ctx context.Context, body []byte, status int) {
	// No response was written if processing panicked. Never report that as success.
	if status == 0 {
		status = http.StatusInternalServerError
	}
	e := nswaudit.Event{
		EventType: nswaudit.EventCustoms, Action: nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetCustomsCallback, Failure: status >= 400,
		Metadata: map[string]any{"source": "slce", "httpStatus": status},
	}
	var identity callbackIdentity
	if err := json.Unmarshal(body, &identity); err != nil {
		e.Metadata["error_code"] = "invalid_callback_payload"
		h.auditor.Audit(ctx, e)
		return
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
	h.auditor.Audit(ctx, e)
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
