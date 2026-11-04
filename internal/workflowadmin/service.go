// Package workflowadmin is the ops/admin view of the workflows a deployment runs, the same
// in TNSW and in an agency: a workflow's raw engine state, and the admin's overrides of a
// node parked for them or forced to park (resolve, force-park). It addresses workflows by
// ID and checks no ownership of their case: its routes are gated on the admin scopes.
package workflowadmin

import (
	"context"

	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
)

// TaskStore is the narrow interface needed from the task store. GetTasks is passed only a
// CaseID, so every task of the case is listed whatever its read roles: this is an admin view.
type TaskStore interface {
	GetTasks(ctx context.Context, filter store.TaskFilter) []store.TaskRecord
	GetTaskByWorkflowID(ctx context.Context, workflowID string) (store.TaskRecord, bool)
}

// TaskCanceller closes a task from outside its workflow. core's orchestrator.TaskManager implements
// it.
type TaskCanceller interface {
	// CancelTask marks the task cancelled and terminates its workflow. It returns an error wrapping
	// orchestrator.ErrTaskNotOpen if the task has completed or does not exist.
	CancelTask(ctx context.Context, taskID, reason string) error
}

// Service reads and overrides workflows through the deployment's two workflow managers.
type Service struct {
	// wm is the flow (parent) workflow manager: a case's flows and their child workflows.
	wm workflow.Manager
	// taskWm is the workflow.Manager for the per-task ("micro") workflow runner — a separate
	// manager/task queue from wm.
	taskWm workflow.Manager
	// tasks closes the task of a force-parked TASK node.
	tasks TaskCanceller
	// taskStore links a TASK node to its task workflow, and a task workflow to its case.
	taskStore TaskStore
}

// NewService creates a Service. A nil dependency only fails the operations that need it.
func NewService(wm, taskWm workflow.Manager, tasks TaskCanceller, taskStore TaskStore) *Service {
	return &Service{wm: wm, taskWm: taskWm, tasks: tasks, taskStore: taskStore}
}
