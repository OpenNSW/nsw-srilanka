package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"
)

// The events SLPA's CMS sends as a service order moves through its approvals.
// An order is raised, reviewed by the accounts clerk, then by the accountant;
// either can refuse it, and the accountant can send it back to the clerk.
const (
	EventApprovedByAccountsClerk = "service_order.approved_by_accounts_clerk"
	EventRejectedByAccountsClerk = "service_order.rejected_by_accounts_clerk"
	EventApprovedByAccountant    = "service_order.approved_by_accountant"
	EventRejectedByAccountant    = "service_order.rejected_by_accountant"
	EventRejectedToAccountsClerk = "service_order.rejected_to_accounts_clerk"
)

// What an event settles. Only the accountant's decisions are final: the clerk's
// approval and a return to the clerk leave the order with SLPA, so the trader is
// shown where it is and the step stays open.
const (
	DecisionPending  = "PENDING"
	DecisionApproved = "APPROVED"
	DecisionRejected = "REJECTED"
)

// Event is the part of the CMS's webhook body this integration acts on.
//
// Their payload also carries the whole order — invoice details, payment slips,
// line items, receipts — which is theirs to hold and would go stale here the
// moment anything changed on their side. What is kept is the correlator (the
// slug recorded when the order was raised), what the event settles, and the few
// identifiers a trader quotes: the order number, and the invoice once it exists.
type OrderEvent struct {
	Event          string  `json:"event"`
	Slug           string  `json:"slug"`
	ServiceOrderNo string  `json:"service_order_no"`
	CusdecSerial   string  `json:"cusdec_serial"`
	Status         string  `json:"status"`
	InvoiceNo      string  `json:"invoice_no"`
	TotalAmount    float64 `json:"total_amount"`
	Timestamp      string  `json:"timestamp"`
}

// Validate reports whether the event can be acted on at all.
func (e OrderEvent) Validate() error {
	if strings.TrimSpace(e.Slug) == "" {
		return fmt.Errorf("slpa webhook: a service order event needs a slug")
	}
	if e.Decision() == "" {
		return fmt.Errorf("%w: %q", ErrUnknownEvent, e.Event)
	}
	return nil
}

// Decision reports what the event settles, or "" for an event not modelled here.
func (e OrderEvent) Decision() string {
	switch e.Event {
	case EventApprovedByAccountant:
		return DecisionApproved
	case EventRejectedByAccountsClerk, EventRejectedByAccountant:
		return DecisionRejected
	case EventApprovedByAccountsClerk, EventRejectedToAccountsClerk:
		return DecisionPending
	default:
		return ""
	}
}

// Stage describes where the order stands, in the words a trader can act on
// rather than the CMS's internal status codes ("actclk_approve_act").
func (e OrderEvent) Stage() string {
	switch e.Event {
	case EventApprovedByAccountsClerk:
		return "Approved by the accounts clerk, with the accountant"
	case EventRejectedToAccountsClerk:
		return "Returned by the accountant to the accounts clerk"
	case EventApprovedByAccountant:
		return "Approved by the accountant"
	case EventRejectedByAccountsClerk:
		return "Rejected by the accounts clerk"
	case EventRejectedByAccountant:
		return "Rejected by the accountant"
	default:
		return ""
	}
}

// ApprovalWaitTemplateID is the subtask the approval-tracking task parks on while
// SLPA's clerks work through the order. Raising an order and waiting on it are
// separate tasks: the first is done when the CMS answers, and what follows is a
// review the trader watches rather than a step they are in the middle of.
//
// This service resumes that subtask and no other, so a stray decision cannot
// complete some unrelated step.
const ApprovalWaitTemplateID = "slpa-approval--wait"

// OrderEvents applies a CMS approval decision to the consignment waiting on it.
type OrderEvents struct {
	lookup taskLookup
	tasks  TaskCompleter
}

// NewOrderEvents binds the service to the task store it reads and the task
// manager it writes through.
func NewOrderEvents(db *gorm.DB, tasks TaskCompleter) *OrderEvents {
	return &OrderEvents{lookup: taskLookup{db: db}, tasks: tasks}
}

// Handle applies what the CMS decided.
//
// Every event completes the waiting step, final or not, because the task manager
// is the only thing that writes to a task record. A decision that leaves the
// order with SLPA is passed on as final:false, and the task workflow's own
// gateway sends it straight back to the same wait — so the trader sees the order
// move ("approved by the accounts clerk, with the accountant") without anything
// reaching behind the framework to edit a record in place.
func (s *OrderEvents) Handle(ctx context.Context, event OrderEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}

	taskID, err := s.lookup.parked(ctx, ApprovalWaitTemplateID, "data->'so'->>'slug' = ?", event.Slug)
	if err != nil {
		return err
	}
	if taskID == "" {
		slog.InfoContext(ctx, "slpa webhook: approval already settled, treating as redelivery",
			"slug", event.Slug, "event", event.Event)
		return nil
	}

	decision := event.Decision()
	payload := map[string]any{
		"__command":        "submit",
		"decision":         decision,
		"final":            decision != DecisionPending,
		"stage":            event.Stage(),
		"cms_status":       event.Status,
		"service_order_no": event.ServiceOrderNo,
		"decided_at":       event.Timestamp,
	}
	if event.InvoiceNo != "" {
		payload["invoice_no"] = event.InvoiceNo
	}
	if event.TotalAmount != 0 {
		payload["total_amount"] = event.TotalAmount
	}

	if err := s.tasks.CompleteTaskStep(ctx, taskID, payload); err != nil {
		return fmt.Errorf("slpa webhook: failed to complete task %s: %w", taskID, err)
	}

	slog.InfoContext(ctx, "slpa webhook: approval applied",
		"task_id", taskID, "slug", event.Slug, "event", event.Event,
		"decision", decision, "final", decision != DecisionPending)
	return nil
}
