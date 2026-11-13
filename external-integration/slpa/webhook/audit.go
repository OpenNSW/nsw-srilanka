package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/felixge/httpsnoop"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// auditActorID names SLPA's CMS as the actor of its deliveries. The route carries
// no IdP token to name it by: the signature is the whole of its authentication.
const auditActorID = "slpa"

// maxAuditIDLen bounds the CMS's identifiers in an audit record, in bytes. A
// longer value is left out, not cut into a different identifier; lookups use the
// value as sent.
const maxAuditIDLen = 128

// WithAuditor has each delivery that passes the signature check recorded as one
// SLPA_EVENT audit record. The order it names is looked up among those the
// consignments recorded after the event is applied; a missing order or a failed
// lookup never changes the response. Configure it before serving requests.
func (h *Handler) WithAuditor(a nswaudit.Auditor) *Handler {
	h.auditor, h.recorded = a, h.orders.lookup
	return h
}

// deliveryIdentity decodes only what names a delivery's event and order. The rest
// of the body, which carries the whole order, its receipts, and the shipper and
// consignee, must not reach the audit record.
type deliveryIdentity struct {
	Event          string `json:"event"`
	Slug           string `json:"slug"`
	ServiceOrderNo string `json:"service_order_no"`
	InvoiceNo      string `json:"invoice_no"`
}

// correlator is what ties the delivery to an order: its slug, or its order number
// when it carries no slug.
func (d deliveryIdentity) correlator() string {
	if slug := strings.TrimSpace(d.Slug); slug != "" {
		return slug
	}
	return strings.TrimSpace(d.ServiceOrderNo)
}

// auditedEvents are the event names a record may carry: the ones this route
// applies. Any other is recorded as UNKNOWN, so a name the sender chose never
// reaches the record.
var auditedEvents = map[string]bool{
	EventApprovedByAccountsClerk: true,
	EventRejectedByAccountsClerk: true,
	EventApprovedByAccountant:    true,
	EventRejectedByAccountant:    true,
	EventRejectedToAccountsClerk: true,
	EventInvoicePaid:             true,
}

// auditDelivery records how one signed delivery was answered. It copies only the
// event name and the order's identifiers out of body.
func (h *Handler) auditDelivery(ctx context.Context, body []byte, status int) {
	// Nothing is written when applying the event panics, and that is no success.
	if status == 0 {
		status = http.StatusInternalServerError
	}
	e := nswaudit.Event{
		EventType: nswaudit.EventSLPA, Action: nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetSLPACallback, Failure: status >= 400,
		ActorType: nswaudit.ActorService, ActorID: auditActorID,
		Metadata: map[string]any{"source": "slpa", "httpStatus": status},
	}
	var id deliveryIdentity
	if err := json.Unmarshal(body, &id); err != nil {
		e.Metadata["error_code"] = "invalid_callback_payload"
		h.auditor.Audit(ctx, e)
		return
	}
	e.Metadata["eventType"] = "UNKNOWN"
	if e.Failure {
		e.Metadata["error_code"] = "callback_rejected"
	}
	putBounded(e.Metadata, "slug", id.Slug)
	putBounded(e.Metadata, "serviceOrderNo", id.ServiceOrderNo)
	putBounded(e.Metadata, "invoiceNo", id.InvoiceNo)
	if auditedEvents[id.Event] {
		e.Metadata["eventType"] = id.Event
		e.TargetType = nswaudit.TargetServiceOrder
		h.resolveOrder(ctx, id, &e)
		if e.TargetID == "" {
			e.Metadata["targetResolutionError"] = "service order could not be resolved"
		}
	}
	h.auditor.Audit(ctx, e)
}

// resolveOrder targets e at the order the delivery names, by the slug and case
// the consignment recorded for it.
func (h *Handler) resolveOrder(ctx context.Context, id deliveryIdentity, e *nswaudit.Event) {
	correlator := id.correlator()
	if h.recorded == nil || correlator == "" {
		return
	}
	order, ok, err := h.recorded.recordedOrder(ctx, correlator)
	if err != nil {
		slog.WarnContext(ctx, "slpa webhook: audit order lookup failed", "error", err)
		return
	}
	if ok {
		e.TargetID = order.Slug
		e.Metadata["caseId"] = order.CaseID
	}
}

// putBounded sets key to value when value is set and fits maxAuditIDLen.
func putBounded(metadata map[string]any, key, value string) {
	if value != "" && len(value) <= maxAuditIDLen {
		metadata[key] = value
	}
}

// recordStatus wraps w so that *status holds the first status written through it,
// an implicit 200 included. The response itself is unchanged.
func recordStatus(w http.ResponseWriter, status *int) http.ResponseWriter {
	return httpsnoop.Wrap(w, httpsnoop.Hooks{
		WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				if *status == 0 {
					*status = code
				}
				next(code)
			}
		},
		Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return func(p []byte) (int, error) {
				if *status == 0 {
					*status = http.StatusOK
				}
				return next(p)
			}
		},
	})
}
