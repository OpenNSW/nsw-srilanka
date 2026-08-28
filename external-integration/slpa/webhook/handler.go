package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/httputil"
)

const (
	errInvalidRequestPayload = "invalid request payload"
	errUnauthorized          = "signature verification failed"
	errUnknownEvent          = "unknown or unsupported event type"
	errOrderNotFound         = "no service order with that slug"
	errNotWaitingYet         = "the order is not waiting on this yet, retry shortly"
)

// maxWebhookBody bounds what will be read from the CMS. Their payload carries
// the whole order — invoice, line items, receipts — so this is generous, but a
// body still cannot be unbounded: the signature is computed over what is read.
const maxWebhookBody = 1 << 20 // 1 MB

// Handler receives the CMS's service-order webhook at POST /webhooks/slpa.
//
// Unlike the OGA portals, which authenticate with a token from the shared IdP,
// SLPA signs each call with a secret shared out of band — so this route carries
// no bearer token and the signature is the whole of its authentication. See
// VerifySignature.
type Handler struct {
	orders   *OrderEvents
	invoices *InvoiceEvents
	secret   string
}

// NewHandler binds the handler to the service that applies a decision. A blank
// secret is refused: an unauthenticated webhook on a live cargo system would let
// anyone advance another trader's consignment.
func NewHandler(orders *OrderEvents, invoices *InvoiceEvents, secret string) (*Handler, error) {
	if orders == nil {
		return nil, errors.New("slpa webhook: service order webhook service is required")
	}
	if invoices == nil {
		return nil, errors.New("slpa webhook: invoice webhook service is required")
	}
	if secret == "" {
		return nil, errors.New("slpa webhook: signing secret is required")
	}
	return &Handler{orders: orders, invoices: invoices, secret: secret}, nil
}

// HandleWebhook authenticates the call, then applies what the CMS decided.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBody)
	defer func() { _ = r.Body.Close() }()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.WarnContext(r.Context(), "slpa webhook: failed to read request body", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	// Before anything is decoded: an unauthenticated body is not worth parsing,
	// and the signature is over the bytes as received.
	if err := VerifySignature(r.Header.Get(SignatureHeader), body, h.secret); err != nil {
		slog.WarnContext(r.Context(), "slpa webhook: rejected an unauthenticated call", "error", err)
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}

	// One signed route carries both lifecycles, as the CMS sends them: the
	// approvals a service order goes through, and the invoice raised once it is
	// approved. Which envelope this is decides only who reads it.
	var envelope struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		slog.WarnContext(r.Context(), "slpa webhook: failed to decode the event", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestPayload)
		return
	}

	var applyErr error
	switch {
	case strings.HasPrefix(envelope.Event, "service_order."):
		applyErr = h.applyOrderEvent(r, body)
	case strings.HasPrefix(envelope.Event, "invoice."):
		applyErr = h.applyInvoiceEvent(r, body)
	default:
		slog.WarnContext(r.Context(), "slpa webhook: event not handled here", "event", envelope.Event)
		httputil.Error(w, r, http.StatusBadRequest, errUnknownEvent)
		return
	}

	switch {
	case applyErr == nil:
		httputil.JSON(w, http.StatusOK, map[string]any{"received": true})

	case errors.Is(applyErr, ErrUnknownEvent):
		// Answered rather than retried: a redelivery would carry the same event
		// and fail the same way.
		slog.WarnContext(r.Context(), "slpa webhook: event not handled here", "event", envelope.Event)
		httputil.Error(w, r, http.StatusBadRequest, errUnknownEvent)

	case errors.Is(applyErr, ErrOrderNotFound):
		// The order was raised somewhere else, or has not been recorded yet. A
		// 404 lets the CMS retry, which is what a race with our own write wants.
		slog.WarnContext(r.Context(), "slpa webhook: no order for this event", "event", envelope.Event)
		httputil.Error(w, r, http.StatusNotFound, errOrderNotFound)

	case errors.Is(applyErr, ErrNotWaitingYet):
		// The consignment holds the order but is between steps: the previous
		// event is still being applied. Their retry will land.
		slog.InfoContext(r.Context(), "slpa webhook: order is between steps, asking for a retry", "event", envelope.Event)
		httputil.Error(w, r, http.StatusConflict, errNotWaitingYet)

	default:
		httputil.InternalServerError(w, r, "slpa webhook: failed to apply the event", applyErr,
			"event", envelope.Event)
	}
}

// applyOrderEvent hands a service-order decision to the service that owns it.
func (h *Handler) applyOrderEvent(r *http.Request, body []byte) error {
	var event OrderEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("%w: %v", ErrUnknownEvent, err)
	}
	return h.orders.Handle(r.Context(), event)
}

// applyInvoiceEvent hands an invoice event to the service that owns it.
func (h *Handler) applyInvoiceEvent(r *http.Request, body []byte) error {
	var event InvoiceEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("%w: %v", ErrUnknownEvent, err)
	}
	return h.invoices.Handle(r.Context(), event)
}
