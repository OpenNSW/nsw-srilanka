package consignment

import (
	"context"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/taskflow/orchestrator"
	workflow "github.com/OpenNSW/core/workflow"
)

// ErrNodeNotWaiting is returned by the force-park methods when the step names no TASK node that is
// still waiting on it: the step is unknown or over, the node is already parked, or its task finished
// while the request was in flight.
var ErrNodeNotWaiting = errors.New("node is not waiting on its step")

// ErrForceParkUnsupportedNodeType is returned by the force-park methods for a node that isn't a TASK.
var ErrForceParkUnsupportedNodeType = errors.New("force-park is supported only for TASK nodes")

// TaskCanceller closes a task from outside its workflow. core's orchestrator.TaskManager implements
// it.
type TaskCanceller interface {
	// CancelTask marks the task cancelled and terminates its workflow. It returns an error wrapping
	// orchestrator.ErrTaskNotOpen if the task has completed or does not exist.
	CancelTask(ctx context.Context, taskID, reason string) error
}

// RegisterTaskCanceller registers what ForceParkTaskNode closes a task with.
func (s *Service) RegisterTaskCanceller(c TaskCanceller) error {
	if s.tasks != nil {
		return fmt.Errorf("task canceller already registered for ConsignmentService")
	}
	if c == nil {
		return fmt.Errorf("task canceller cannot be nil")
	}
	s.tasks = c
	return nil
}

// ForceParkTaskNode moves a TASK node on a consignment workflow that is waiting on its task into
// AWAITING_ADMIN, and closes that task. An admin then resolves the node like any parked node: RETRY
// starts a new task under a new step, COMPLETE moves on with a variables patch as the node's output,
// ABORT fails the workflow. Use it when the whole task has to go, for example to rebuild it from
// amended data. To override a single step and let the task carry on, use ForceParkTaskWorkflowStep.
//
// workflowID is the consignment's root workflow or one of its child-branch workflows, and stepID is
// the node's step_id from that workflow's engine status. It returns the node's ID in the workflow
// definition, for the caller's audit record.
//
// The node's step is parked first, so the node enters AWAITING_ADMIN (with ParkCategoryAdminForced)
// and stops accepting the task's completion, and then the task is closed. If closing the task fails,
// the node is already visibly parked and repeating the request finishes closing the task.
func (s *Service) ForceParkTaskNode(ctx context.Context, workflowID, stepID, reason string) (nodeID string, err error) {
	if s.tasks == nil {
		return "", fmt.Errorf("no task canceller registered for ConsignmentService")
	}
	return forceParkStep(ctx, s.wm, workflowID, stepID, reason, func() error {
		// The task is named after the step that started it (see core's TaskManager.StartTask). If
		// the task completed in the gap after ForceParkActivation, its completion was rejected by
		// the parent workflow (ErrActivationNotPending) and the task is already closed.
		err := s.tasks.CancelTask(ctx, stepID, reason)
		if errors.Is(err, orchestrator.ErrTaskNotOpen) {
			return nil
		}
		return err
	})
}

// ForceParkTaskWorkflowStep moves one step of a task workflow that is waiting, such as a wait for an
// external callback, into AWAITING_ADMIN. The task stays open. Resolving the step through the task
// workflow's resolve route lets the task carry on: COMPLETE with a patch of the variables the step
// would have produced stands in for a callback that will never come, and the task then ends
// normally and reports to its parent.
//
// taskWorkflowID is a TASK node's task_workflow_id and stepID is the step's step_id from that task
// workflow's engine status. It returns the step's node ID, for the caller's audit record.
//
// While the step is parked the task still shows the step's own state, and a submission for it is
// rejected as stale.
func (s *Service) ForceParkTaskWorkflowStep(ctx context.Context, taskWorkflowID, stepID, reason string) (nodeID string, err error) {
	return forceParkStep(ctx, s.taskWm, taskWorkflowID, stepID, reason, nil)
}

// forceParkStep is the body both force-park methods share, run against whichever manager owns
// workflowID. It checks the step names a TASK node waiting on it, parks the step first, then runs
// afterPark if set. If an earlier call parked the node (ParkCategoryAdminForced) and failed in
// afterPark, a retry skips ForceParkActivation and finishes afterPark.
func forceParkStep(ctx context.Context, mgr workflow.Manager, workflowID, stepID, reason string, afterPark func() error) (string, error) {
	if mgr == nil {
		return "", fmt.Errorf("no workflow manager registered for workflow %s", workflowID)
	}
	parker, ok := mgr.(workflow.ActivationParker)
	if !ok {
		return "", fmt.Errorf("workflow manager does not support force-parking a step")
	}

	instance, err := mgr.GetStatus(ctx, workflowID)
	if err != nil {
		if errors.Is(err, workflow.ErrWorkflowNotFound) {
			return "", ErrEngineWorkflowNotFound
		}
		return "", fmt.Errorf("failed to verify step %s is waiting on workflow %s: %w", stepID, workflowID, err)
	}
	nodeID, alreadyForceParked, err := findForceParkNode(instance.NodeInfo, stepID, afterPark != nil)
	if err != nil {
		return "", err
	}

	if !alreadyForceParked {
		if err := parker.ForceParkActivation(ctx, workflowID, "", stepID, reason); err != nil {
			if errors.Is(err, workflow.ErrActivationNotPending) {
				return "", fmt.Errorf("%w: %w", ErrNodeNotWaiting, err)
			}
			return "", fmt.Errorf("failed to park node %s on workflow %s: %w", nodeID, workflowID, err)
		}
	}
	if afterPark != nil {
		if err := afterPark(); err != nil {
			return "", err
		}
	}
	return nodeID, nil
}

// findForceParkNode locates the TASK node whose current run is stepID and checks whether it is
// waiting (or, when allowAlreadyForceParked is true, already force-parked so a retry can finish
// closing its task).
func findForceParkNode(nodeInfo map[string]*workflow.NodeInfo, stepID string, allowAlreadyForceParked bool) (nodeID string, alreadyForceParked bool, err error) {
	nodeID, node, ok := findNodeByStep(nodeInfo, stepID)
	if !ok {
		return "", false, ErrNodeNotWaiting
	}
	if node.Type != workflow.NodeTypeTask {
		return "", false, ErrForceParkUnsupportedNodeType
	}
	alreadyForceParked = allowAlreadyForceParked &&
		node.Status == workflow.NodeStatusAwaitingAdmin &&
		node.ParkCategory == workflow.ParkCategoryAdminForced
	if node.Status != workflow.NodeStatusRunning && !alreadyForceParked {
		return "", false, ErrNodeNotWaiting
	}
	return nodeID, alreadyForceParked, nil
}

// findNodeByStep finds the node whose current run is stepID, whatever its status.
func findNodeByStep(nodeInfo map[string]*workflow.NodeInfo, stepID string) (nodeID string, node *workflow.NodeInfo, ok bool) {
	if stepID == "" {
		return "", nil, false
	}
	for id, n := range nodeInfo {
		if n.ActivationID == stepID {
			return id, n, true
		}
	}
	return "", nil, false
}
