// Package parkedtask finds the task an SLC Edge callback answers: the one a
// customs workflow parked on a template while ASYCUDA holds the next move.
//
// It reads tasks through core's task model, by the case and, for a wait in the
// workflow the dispatch was made from, that workflow: the declaration or
// dispatch note recorded both when SLC Edge accepted it. Nothing here reads a
// task's data.
package parkedtask

import (
	"context"
	"fmt"

	"github.com/OpenNSW/core/taskflow/store"
	gormstore "github.com/OpenNSW/core/taskflow/store/gorm"
	"gorm.io/gorm"
)

// StateQueuedExternally is the state a task parks in while an external system
// owns the next move. Only a task in that state is waiting to be resumed.
const StateQueuedExternally = "QUEUED_EXTERNALLY"

// Tasks loads the tasks of a case. A load that fails returns its error: a
// callback must not read a failed load as "nothing is waiting", or it would be
// acknowledged with the task still parked and never retried.
type Tasks interface {
	TasksOfCase(ctx context.Context, caseID string) ([]store.TaskRecord, error)
}

// Store loads tasks from the task table through core's task model.
//
// core's task store has GetAllTasks, but it logs a failed query and answers
// with no tasks, which is the one case Tasks has to tell apart.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store reading from db.
func NewStore(db *gorm.DB) Store { return Store{db: db} }

// TasksOfCase returns the tasks of caseID, or the error that kept them from
// being read.
func (s Store) TasksOfCase(ctx context.Context, caseID string) ([]store.TaskRecord, error) {
	var models []gormstore.TaskRecordModel
	if err := s.db.WithContext(ctx).Where("case_id = ?", caseID).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load the tasks of case %s: %w", caseID, err)
	}
	records := make([]store.TaskRecord, len(models))
	for i, m := range models {
		records[i] = m.ToDomain()
	}
	return records, nil
}

// Task names the parked task and the step it is parked on. The callback
// completes exactly that step, so one that has since moved on is not touched.
type Task struct {
	TaskID string
	StepID string
}

// Find returns the task of workflow workflowID, in case caseID, that is parked
// on templateID, and whether there is one. A case runs one workflow per flow, so
// at most one of its tasks waits on a given template in a given workflow.
//
// An error means the tasks could not be read, so whether one is parked is not
// known; false with no error means none is.
func Find(ctx context.Context, tasks Tasks, caseID, workflowID, templateID string) (Task, bool, error) {
	if workflowID == "" {
		return Task{}, false, nil
	}
	return find(ctx, tasks, caseID, func(rec store.TaskRecord) bool {
		return rec.ParentWorkflowID == workflowID && rec.ActiveTaskTemplateID == templateID
	})
}

// FindInCase returns a task of case caseID parked on templateID in any of the
// case's workflows, and whether there is one. It is for a wait that runs in
// another workflow than the dispatch did: a parallel split runs each branch as
// a child workflow with an ID of its own, so a wait after the split never has
// the dispatch step's workflow as its parent, while the case is shared by every
// flow of the consignment at every level of nesting. Use it only for a
// template a case parks on once at a time.
//
// The error and false mean what they do for Find.
func FindInCase(ctx context.Context, tasks Tasks, caseID, templateID string) (Task, bool, error) {
	return find(ctx, tasks, caseID, func(rec store.TaskRecord) bool {
		return rec.ActiveTaskTemplateID == templateID
	})
}

// find returns the first parked task of caseID that match accepts.
func find(ctx context.Context, tasks Tasks, caseID string, match func(store.TaskRecord) bool) (Task, bool, error) {
	if caseID == "" {
		return Task{}, false, nil
	}
	records, err := tasks.TasksOfCase(ctx, caseID)
	if err != nil {
		return Task{}, false, err
	}
	for _, rec := range records {
		if rec.State == StateQueuedExternally && match(rec) {
			return Task{TaskID: rec.TaskID, StepID: rec.ActiveStepID}, true, nil
		}
	}
	return Task{}, false, nil
}
