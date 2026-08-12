package payment

import (
	"net/http"

	corepayment "github.com/OpenNSW/core/payment"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// AuditedHandler wraps the core payment HTTPHandler and records audit events
// at the handler layer.
type AuditedHandler struct {
	inner *corepayment.HTTPHandler
	audit *nswaudit.Recorder
}

func NewAuditedHandler(inner *corepayment.HTTPHandler, recorder *nswaudit.Recorder) *AuditedHandler {
	return &AuditedHandler{inner: inner, audit: recorder}
}

func (h *AuditedHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gatewayID := r.PathValue("gatewayId")

	sw := &nswaudit.StatusWriter{ResponseWriter: w}
	h.inner.HandleWebhook(sw, r)

	h.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventPayment,
		Action:     nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetPayment,
		TargetID:   gatewayID,
		Failure:    nswaudit.HTTPStatus(sw.Status) >= http.StatusBadRequest,
		Metadata: map[string]any{
			"status":    nswaudit.HTTPStatus(sw.Status),
			"gatewayId": gatewayID,
		},
	})
}

func (h *AuditedHandler) HandleValidateReference(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gatewayID := r.PathValue("gatewayId")

	sw := &nswaudit.StatusWriter{ResponseWriter: w}
	h.inner.HandleValidateReference(sw, r)

	h.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventPayment,
		Action:     nswaudit.ActionRead,
		TargetType: nswaudit.TargetPayment,
		TargetID:   gatewayID,
		Failure:    nswaudit.HTTPStatus(sw.Status) >= http.StatusBadRequest,
		Metadata: map[string]any{
			"status":    nswaudit.HTTPStatus(sw.Status),
			"gatewayId": gatewayID,
		},
	})
}
