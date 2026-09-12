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

// InvoiceEvent is the part of the CMS's invoice webhook this integration acts on.
//
// Their live payload is flat: the receipt sits on the envelope as
// payment_receipt, and the invoice it settles is named beside it. An earlier
// draft of the contract nested the same facts under details.invoice_details, so
// both are read — a deployment still sending the older shape must not silently
// settle a step with nothing on it.
//
// Two fields are traps, and are read through the methods below rather than
// directly. total_amount on the envelope is the dollar figure, not what was
// transferred; payment_receipt.payment_receipt is a receipt number, not a link
// to one.
type InvoiceEvent struct {
	Event          string  `json:"event"`
	Slug           string  `json:"slug"`
	Status         string  `json:"status"`
	ServiceOrderNo string  `json:"service_order_no"`
	InvoiceNo      string  `json:"invoice_no"`
	CusdecSerial   string  `json:"cusdec_serial"`
	TotalAmount    float64 `json:"total_amount"`
	InvoiceURLFlat string  `json:"invoice_url"`
	Timestamp      string  `json:"timestamp"`

	// Receipt is what the CMS issues when the money lands, on the envelope.
	Receipt struct {
		InvoiceNo      string  `json:"invoice_no"`
		InvoiceSerial  string  `json:"invoice_serial"`
		PaidAmount     float64 `json:"paid_amount"`
		PaidDateTime   string  `json:"paid_datetime"`
		ReceiptNo      string  `json:"payment_receipt"`
		ServiceOrderNo string  `json:"service_order_no"`
	} `json:"payment_receipt"`

	Details struct {
		InvoiceDetails struct {
			InvoiceNo       string  `json:"invoice_no"`
			InvoiceSerial   string  `json:"invoice_serial"`
			Status          string  `json:"status"`
			TotalLKR        float64 `json:"total_lkr"`
			TotalPayableLKR float64 `json:"total_payable_lkr"`
			ExchangeRate    float64 `json:"exchange_rate"`
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

// Number is the invoice number, from wherever the CMS put it.
func (e InvoiceEvent) Number() string {
	if no := strings.TrimSpace(e.InvoiceNo); no != "" {
		return no
	}
	return strings.TrimSpace(e.Details.InvoiceDetails.InvoiceNo)
}

// InvoiceURL is where the trader downloads the invoice to pay against.
func (e InvoiceEvent) InvoiceURL() string {
	return firstLink(e.InvoiceURLFlat, e.Details.InvoiceDetails.InvoiceURL)
}

// ReceiptURL is where the trader downloads the proof that it was paid, when the
// CMS sends a link at all — their live payment sends none, and the receipt
// number stands in.
//
// Only values that are links are offered. payment_receipt was read here once,
// which put a bare "100415624" behind a Download link that could go nowhere.
func (e InvoiceEvent) ReceiptURL() string {
	return firstLink(
		e.Details.InvoiceDetails.PaymentSlipURL,
		e.Details.InvoiceDetails.InvoiceURL,
		e.InvoiceURLFlat,
	)
}

// firstLink returns the first value that is a URL, so a reference number never
// reaches the panel as something to click.
func firstLink(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v
		}
	}
	return ""
}

// Payable is what was actually transferred, in rupees.
//
// Every candidate here is a rupee figure. The envelope's total_amount is not:
// their live payment carries total_amount 16 beside a paid_amount of 4776, which
// is the order priced in dollars next to the rupees that left the account. Read
// as a fallback it would put "LKR 16.00" on a settled panel, so it is not read
// at all — a missing amount shows nothing, which is the honest outcome.
func (e InvoiceEvent) Payable() float64 {
	d := e.Details.InvoiceDetails
	for _, amount := range []float64{
		e.Receipt.PaidAmount,
		d.TotalPayableLKR,
		d.PaymentReceipt.PaidAmount,
		d.TotalLKR,
	} {
		if amount != 0 {
			return amount
		}
	}
	return 0
}

// ReceiptNo is the reference the CMS issues against the payment — "100415624".
// It is not a document: SLPA sends no link to one on this event, and the trader
// quotes this number at the terminal instead.
func (e InvoiceEvent) ReceiptNo() string {
	return firstOf(e.Receipt.ReceiptNo, e.Details.InvoiceDetails.PaymentReceipt.PaymentReceipt)
}

// Serial is the invoice serial, from wherever the CMS put it.
func (e InvoiceEvent) Serial() string {
	return firstOf(e.Receipt.InvoiceSerial, e.Details.InvoiceDetails.InvoiceSerial)
}

// PaidAt is when the money moved.
func (e InvoiceEvent) PaidAt() string {
	return firstOf(
		e.Receipt.PaidDateTime,
		e.Details.InvoiceDetails.PaidAt,
		e.Details.InvoiceDetails.PaymentReceipt.PaidDateTime,
		e.Timestamp,
	)
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

	details := event.Details.InvoiceDetails
	payload := map[string]any{
		"__command":        "submit",
		"paid":             true,
		"invoice_no":       event.Number(),
		"service_order_no": firstOf(event.ServiceOrderNo, event.Receipt.ServiceOrderNo),
		"paid_at":          event.PaidAt(),
	}

	// Sent only when the CMS did. Completing the step rewrites what it recorded,
	// so what the payment restates is what the settled panel keeps — but a field
	// they left out must not blank one the trader could read a moment ago.
	putIf(payload, "cms_status", firstOf(event.Status, details.Status))
	putIf(payload, "invoice_serial", event.Serial())
	putIf(payload, "receipt_no", event.ReceiptNo())
	putIf(payload, "receipt_url", event.ReceiptURL())
	putIf(payload, "invoice_url", event.InvoiceURL())
	putIf(payload, "payment_slip_url", details.PaymentSlipURL)
	putIf(payload, "generated_at", details.GeneratedAt)
	if payable := event.Payable(); payable != 0 {
		payload["payable_lkr"] = payable
	}
	if details.ExchangeRate != 0 {
		payload["exchange_rate"] = details.ExchangeRate
	}

	if err := s.tasks.CompleteTaskStep(ctx, taskID, payload); err != nil {
		return fmt.Errorf("slpa webhook: failed to complete task %s: %w", taskID, err)
	}

	slog.InfoContext(ctx, "slpa webhook: invoice paid",
		"task_id", taskID, "invoice_no", event.Number(), "correlator", correlator)
	return nil
}

// putIf records a value on the payload only when the CMS sent one.
func putIf(payload map[string]any, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		payload[key] = v
	}
}
