package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"
)

// The events the CMS sends once an order is approved. What SLPA bills for is
// settled outside the Single Window — the trader transfers the money to SLPA's
// account — so these two events are all this side knows about it.
const (
	EventInvoiceGenerated = "invoice.generated"
	EventInvoicePaid      = "invoice.paid"
)

// InvoiceEvent is the part of the CMS's invoice webhook this integration acts on.
//
// The documents are the point of these events, and the CMS reports them in two
// places: the flat fields it puts on the envelope, and the invoice_details block
// inside the order it echoes. Both are read — see InvoiceURL and ReceiptURL —
// because which one carries a URL is theirs to decide, and a missing link is the
// one thing that makes this step useless to a trader.
type InvoiceEvent struct {
	Event          string  `json:"event"`
	Slug           string  `json:"slug"`
	ServiceOrderNo string  `json:"service_order_no"`
	InvoiceNo      string  `json:"invoice_no"`
	TotalAmount    float64 `json:"total_amount"`
	InvoiceURLFlat string  `json:"invoice_url"`
	Timestamp      string  `json:"timestamp"`

	Details struct {
		InvoiceDetails struct {
			InvoiceNo       string  `json:"invoice_no"`
			InvoiceSerial   string  `json:"invoice_serial"`
			Status          string  `json:"status"`
			TotalUSD        float64 `json:"total_usd"`
			TotalLKR        float64 `json:"total_lkr"`
			TotalPayableLKR float64 `json:"total_payable_lkr"`
			InvoiceURL      string  `json:"invoice_url"`
			PaymentSlipURL  string  `json:"payment_slip_url"`
			GeneratedAt     string  `json:"invoice_generated_at"`
			PaidAt          string  `json:"invoice_paid_at"`

			PaymentReceipt struct {
				PaymentReceipt string  `json:"payment_receipt"`
				PaidAmount     float64 `json:"paid_amount"`
				PaidDateTime   string  `json:"paid_datetime"`
			} `json:"payment_receipt"`
		} `json:"invoice_details"`
	} `json:"details"`
}

// Validate reports whether the event can be acted on at all.
func (e InvoiceEvent) Validate() error {
	if e.Event != EventInvoiceGenerated && e.Event != EventInvoicePaid {
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

// Number is the invoice number, from wherever the CMS put it.
func (e InvoiceEvent) Number() string {
	if no := strings.TrimSpace(e.InvoiceNo); no != "" {
		return no
	}
	return strings.TrimSpace(e.Details.InvoiceDetails.InvoiceNo)
}

// InvoiceURL is where the trader downloads the invoice to pay against.
func (e InvoiceEvent) InvoiceURL() string {
	return firstOf(e.InvoiceURLFlat, e.Details.InvoiceDetails.InvoiceURL)
}

// ReceiptURL is where the trader downloads the proof that it was paid. The CMS
// calls it a payment slip in one place and a receipt in another; the invoice
// itself is the last resort, since a paid invoice is stamped as paid.
func (e InvoiceEvent) ReceiptURL() string {
	return firstOf(
		e.Details.InvoiceDetails.PaymentSlipURL,
		e.Details.InvoiceDetails.PaymentReceipt.PaymentReceipt,
		e.Details.InvoiceDetails.InvoiceURL,
		e.InvoiceURLFlat,
	)
}

// Payable is what the trader owes, preferring the figure the CMS says is
// payable over the currency subtotals.
func (e InvoiceEvent) Payable() float64 {
	d := e.Details.InvoiceDetails
	for _, amount := range []float64{d.TotalPayableLKR, e.TotalAmount, d.TotalLKR, d.TotalUSD} {
		if amount != 0 {
			return amount
		}
	}
	return 0
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
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

// Handle records the invoice, and closes the step once it has been paid.
//
// Both events complete the waiting step, because the task manager is the only
// thing that writes to a task record. The invoice being raised is passed on as
// paid:false and the task workflow's gateway sends it back to the same wait: the
// trader can then see what they owe and download the document, while the step
// stays open until the money has actually moved.
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

	paid := event.Event == EventInvoicePaid
	payload := map[string]any{
		"__command":        "submit",
		"paid":             paid,
		"invoice_no":       event.Number(),
		"invoice_url":      event.InvoiceURL(),
		"payable":          event.Payable(),
		"cms_status":       event.Details.InvoiceDetails.Status,
		"service_order_no": event.ServiceOrderNo,
	}
	if serial := event.Details.InvoiceDetails.InvoiceSerial; serial != "" {
		payload["invoice_serial"] = serial
	}
	if paid {
		payload["receipt_url"] = event.ReceiptURL()
		payload["paid_at"] = firstOf(
			event.Details.InvoiceDetails.PaidAt,
			event.Details.InvoiceDetails.PaymentReceipt.PaidDateTime,
			event.Timestamp,
		)
	} else {
		payload["generated_at"] = firstOf(event.Details.InvoiceDetails.GeneratedAt, event.Timestamp)
	}

	if err := s.tasks.CompleteTaskStep(ctx, taskID, payload); err != nil {
		return fmt.Errorf("slpa webhook: failed to complete task %s: %w", taskID, err)
	}

	slog.InfoContext(ctx, "slpa webhook: invoice event applied",
		"task_id", taskID, "invoice_no", event.Number(), "correlator", correlator, "paid", paid)
	return nil
}
