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

	// Nothing parked under that template. Which of the three cases this is
	// decides whether the CMS should retry.
	var states []string
	if err := l.db.WithContext(ctx).
		Table("task_records_v2").
		Where(match, args...).
		Pluck("state", &states).Error; err != nil {
		return "", fmt.Errorf("slpa webhook: failed to look up the order: %w", err)
	}
	if len(states) == 0 {
		return "", ErrOrderNotFound
	}
	for _, state := range states {
		if state != stateCompleted {
			// A task of this consignment is still running, so the wait is either
			// about to open or has already moved on. Either way a retry is
			// worth its while.
			return "", ErrNotWaitingYet
		}
	}
	// Everything finished: this event has been applied already.
	return "", nil
}
