package webhook

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ErrUnknownEvent is returned for an event this package does not model, so the
// caller can answer the CMS rather than acting on a guess.
var ErrUnknownEvent = errors.New("slpa webhook: unknown event")

// ErrOrderNotFound is returned when no consignment here holds the order an event
// is about.
var ErrOrderNotFound = errors.New("slpa webhook: no order for this event")

// ErrNotWaitingYet is returned when the consignment holds the order but nothing
// is parked on it at this instant — the previous event is still being applied.
// The CMS should retry, which is why this is distinguished from a redelivery.
var ErrNotWaitingYet = errors.New("slpa webhook: the order is not waiting on this yet")

// TaskCompleter releases a task parked on an external event. It is the only way
// this package writes to a task: a record's data belongs to the task manager,
// which persists it as part of resuming the step and lets the workflow decide
// what happens next.
//
// That is why an event which does not end a lifecycle still completes the step:
// the workflow's own gateway sends it back to the same wait, so what the trader
// sees is updated and the step is open again. See the SLPA task workflows.
type TaskCompleter interface {
	CompleteTaskStep(ctx context.Context, taskID string, payload map[string]any) error
}

// stateQueuedExternally is the state a wait parks in while SLPA owns the next
// move. Only a task in that state is waiting to be resumed.
const stateQueuedExternally = "QUEUED_EXTERNALLY"

// stateCompleted is the state a task reaches when its flow has finished.
const stateCompleted = "COMPLETED"

// taskLookup finds the task an event is about. Reading the store this way is
// unavoidable: an event carries the order's identifiers, and nothing but the
// tasks themselves knows which consignment recorded them.
type taskLookup struct {
	db *gorm.DB
}

// parked returns the task waiting on this order under the given wait template.
//
// The three outcomes are deliberately different, because they tell the CMS
// different things: a task to resume, a lifecycle already finished (its retries
// should stop), and an order mid-transition (its retry will land).
//
// Every answer is about this wait alone. A consignment carries the same order
// identifiers on more than one task — the approval and the invoice both hold
// the slug — so a redelivered approval must not be judged by whether the
// invoice task that followed it happens to be open.
func (l taskLookup) parked(ctx context.Context, templateID string, match string, args ...any) (string, error) {
	var task struct {
		TaskID string `gorm:"column:task_id"`
	}
	err := l.db.WithContext(ctx).
		Table("task_records_v2").
		Where("("+match+") AND active_task_template_id = ? AND state = ?",
			append(append([]any{}, args...), templateID, stateQueuedExternally)...).
		Select("task_id").
		First(&task).Error
	if err == nil {
		return task.TaskID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("slpa webhook: failed to look up the order: %w", err)
	}

	// Nothing parked under that wait. Which of the three cases this is decides
	// whether the CMS should retry, and the answer is only ever about this wait:
	// a consignment carries the same order identifiers on several tasks — the
	// approval and the invoice both hold the slug — so reading their states
	// would let a sibling still running answer for a wait that has finished.
	var states []string
	if err := l.db.WithContext(ctx).
		Table("task_records_v2").
		Where("("+match+") AND active_task_template_id = ?", append(append([]any{}, args...), templateID)...).
		Pluck("state", &states).Error; err != nil {
		return "", fmt.Errorf("slpa webhook: failed to look up the order: %w", err)
	}

	for _, state := range states {
		if state == stateCompleted {
			// The wait ran to the end of its flow, so this event has been
			// applied already and the CMS is answered rather than told to
			// retry — which is what stops the retries.
			return "", nil
		}
	}
	if len(states) > 0 {
		// The wait exists but is between steps: the previous event is still
		// being applied and the one after it has not reopened yet. Answering
		// "already applied" here would drop an event that was never acted on,
		// so the CMS is asked to retry, and its retry lands.
		return "", ErrNotWaitingYet
	}

	// This wait has never opened. Whether that is worth retrying depends on
	// there being an order here at all.
	var held int64
	if err := l.db.WithContext(ctx).
		Table("task_records_v2").
		Where(match, args...).
		Count(&held).Error; err != nil {
		return "", fmt.Errorf("slpa webhook: failed to look up the order: %w", err)
	}
	if held == 0 {
		return "", ErrOrderNotFound
	}
	// The consignment holds the order but has not reached this step yet — a
	// decision arriving before the task that waits for it, which their retry
	// resolves.
	return "", ErrNotWaitingYet
}
