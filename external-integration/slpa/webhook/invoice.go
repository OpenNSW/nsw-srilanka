package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"
)

// EventInvoicePaid is the one invoice event this side acts on. What SLPA bills
// for is settled outside the Single Window — the trader transfers the amount to
// SLPA's account — so the payment is theirs to announce.
//
// The invoice itself is not announced: the trader asks for it from the step
// before this one (see the slpa/invoice package), so an invoice.generated
// carries nothing this side does not already hold and is refused like any other
// event the route does not model.
const EventInvoicePaid = "invoice.paid"

// InvoiceEvent is the CMS's invoice webhook, as they send it.
//
// Everything worth keeping sits on payment_receipt: the money that moved, when
// it moved, the reference it moved under, and the link to the receipt itself.
// The envelope carries the correlation fields and little else.
//
// One field is a trap and is not read at all. total_amount on the envelope is
// the order priced in dollars, not what was transferred -- their live payment
// carries total_amount 132 beside a paid_amount of 39402. Reading it, even as a
// last resort, would put "LKR 132.00" on a settled panel.
type InvoiceEvent struct {
	Event          string `json:"event"`
	Slug           string `json:"slug"`
	Status         string `json:"status"`
	ServiceOrderNo string `json:"service_order_no"`
	InvoiceNo      string `json:"invoice_no"`
	CusdecSerial   string `json:"cusdec_serial"`
	Timestamp      string `json:"timestamp"`

	// Receipt is what the CMS issues when the money lands.
	Receipt struct {
		ReceiptNo        string  `json:"payment_receipt"`
		ReceiptURL       string  `json:"payment_receipt_url"`
		PaidAmount       float64 `json:"paid_amount"`
		PaidDateTime     string  `json:"paid_datetime"`
		InvoiceNo        string  `json:"invoice_no"`
		InvoiceSerial    string  `json:"invoice_serial"`
		InvoiceType      string  `json:"invoice_type"`
		ServiceOrderNo   string  `json:"service_order_no"`
		CusdecNo         string  `json:"cusdec_no"`
		Shipper          string  `json:"shipper"`
		Consignee        string  `json:"consignee"`
		ConsigneeAddress string  `json:"consignee_address"`
	} `json:"payment_receipt"`
}

// Validate reports whether the event can be acted on at all.
//
// Only the payment is. An invoice.generated is refused like any event this
// route does not model — the handler answers 400, which tells the CMS the
// redelivery is pointless rather than leaving it to retry.
func (e InvoiceEvent) Validate() error {
	if e.Event != EventInvoicePaid {
		return fmt.Errorf("%w: %q", ErrUnknownEvent, e.Event)
	}
	if e.correlator() == "" {
		return fmt.Errorf("slpa webhook: an invoice event needs a slug or a service order number")
	}
	return nil
}

// correlator is what ties the event to a consignment: the slug the order was
// raised under, or its order number when the CMS sends only that.
func (e InvoiceEvent) correlator() string {
	if slug := strings.TrimSpace(e.Slug); slug != "" {
		return slug
	}
	return strings.TrimSpace(e.ServiceOrderNo)
}

// PaymentWaitTemplateID is the subtask the invoice task parks on, from the moment
// the order is approved until the CMS reports the payment. This service resumes
// that subtask and no other.
const PaymentWaitTemplateID = "slpa-invoice--wait"

// InvoiceEvents applies an invoice event to the consignment waiting on it.
type InvoiceEvents struct {
	lookup taskLookup
	tasks  TaskCompleter
}

// NewInvoiceEvents binds the service to the task store it reads and the task
// manager it writes through.
func NewInvoiceEvents(db *gorm.DB, tasks TaskCompleter) *InvoiceEvents {
	return &InvoiceEvents{lookup: taskLookup{db: db}, tasks: tasks}
}

// Handle closes the waiting step once the invoice has been paid.
//
// The invoice itself was raised by the step before this one, which recorded what
// the trader owes; what arrives here is the confirmation that it was settled,
// and the receipt they keep. Anything the payment answer carries that the
// generate call already recorded is passed on again rather than assumed
// unchanged — the CMS restates the invoice on this event, and a figure that has
// moved is theirs to correct.
func (s *InvoiceEvents) Handle(ctx context.Context, event InvoiceEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}

	correlator := event.correlator()
	taskID, err := s.lookup.parked(ctx, PaymentWaitTemplateID,
		"data->'so'->>'slug' = ? OR data->'so'->>'service_order_no' = ?", correlator, correlator)
	if err != nil {
		return err
	}
	if taskID == "" {
		slog.InfoContext(ctx, "slpa webhook: invoice already settled, treating as redelivery",
			"correlator", correlator, "event", event.Event)
		return nil
	}

	receipt := event.Receipt
	payload := map[string]any{
		"__command":        "submit",
		"paid":             true,
		"invoice_no":       event.InvoiceNo,
		"invoice_serial":   receipt.InvoiceSerial,
		"service_order_no": event.ServiceOrderNo,
		"cms_status":       event.Status,
		"paid_at":          receipt.PaidDateTime,
		"payable_lkr":      receipt.PaidAmount,

		// The receipt proper. It used to fall back to the payment slip or the
		// invoice when no receipt link was found, which put the wrong document
		// behind a "receipt" link -- the CMS does send one, on the receipt
		// block, and that is the only thing read now.
		"receipt_no":  receipt.ReceiptNo,
		"receipt_url": receipt.ReceiptURL,

		// Who paid, and against what. Stated on the settled panel so the
		// payment can be reconciled without opening the PDF.
		"cusdec_no":    receipt.CusdecNo,
		"invoice_type": receipt.InvoiceType,
		"shipper":      receipt.Shipper,
		"consignee":    receipt.Consignee,
	}

	if err := s.tasks.CompleteTaskStep(ctx, taskID, payload); err != nil {
		return fmt.Errorf("slpa webhook: failed to complete task %s: %w", taskID, err)
	}

	slog.InfoContext(ctx, "slpa webhook: invoice paid",
		"task_id", taskID, "invoice_no", event.InvoiceNo, "correlator", correlator)
	return nil
}
