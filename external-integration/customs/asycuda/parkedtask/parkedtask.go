// Package parkedtask finds the task an SLC Edge callback answers: the one a
// customs workflow parked on a template while ASYCUDA holds the next move.
//
// It reads tasks through core's task store, by the case and the workflow the
// dispatch was made from, which the declaration or dispatch note recorded when
// SLC Edge accepted it. Nothing here reads the task table or a task's data.
package parkedtask

import (
	"context"

	"github.com/OpenNSW/core/taskflow/store"
)

// StateQueuedExternally is the state a task parks in while an external system
// owns the next move. Only a task in that state is waiting to be resumed.
const StateQueuedExternally = "QUEUED_EXTERNALLY"

// Tasks is the slice of core's task store the callbacks need. GetAllTasks
// returns the tasks of a case; *gormstore.TaskStore satisfies it.
type Tasks interface {
	GetAllTasks(ctx context.Context, caseID string) []store.TaskRecord
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
func Find(ctx context.Context, tasks Tasks, caseID, workflowID, templateID string) (Task, bool) {
	if caseID == "" || workflowID == "" {
		return Task{}, false
	}
	for _, rec := range tasks.GetAllTasks(ctx, caseID) {
		if rec.ParentWorkflowID == workflowID &&
			rec.ActiveTaskTemplateID == templateID &&
			rec.State == StateQueuedExternally {
			return Task{TaskID: rec.TaskID, StepID: rec.ActiveStepID}, true
		}
	}
	return Task{}, false
}
