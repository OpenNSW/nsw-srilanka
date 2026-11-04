package workflowadmin

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// MockTaskStore implements workflowadmin.TaskStore for testing.
type MockTaskStore struct {
	mock.Mock
}

func (m *MockTaskStore) GetTaskByWorkflowID(ctx context.Context, workflowID string) (store.TaskRecord, bool) {
	args := m.Called(ctx, workflowID)
	return args.Get(0).(store.TaskRecord), args.Bool(1)
}

func (m *MockTaskStore) GetTasks(ctx context.Context, filter store.TaskFilter) []store.TaskRecord {
	args := m.Called(ctx, filter)
	tasks, _ := args.Get(0).([]store.TaskRecord)
	return tasks
}

// MockWM implements workflow.Manager for testing.
type MockWM struct {
	mock.Mock
}

func (m *MockWM) StartWorkflow(ctx context.Context, id string, workflowDefinition workflow.WorkflowDefinition, initialWorkflowVariables map[string]any) error {
	args := m.Called(ctx, id, workflowDefinition, initialWorkflowVariables)
	return args.Error(0)
}

func (m *MockWM) CompleteActivation(ctx context.Context, workflowID, runID, activationID string, output map[string]any) error {
	args := m.Called(ctx, workflowID, runID, activationID, output)
	return args.Error(0)
}

func (m *MockWM) TaskUpdate(ctx context.Context, workflowID, runID string, update workflow.UpdateEvent) error {
	args := m.Called(ctx, workflowID, runID, update)
	return args.Error(0)
}

func (m *MockWM) GetStatus(ctx context.Context, workflowID string) (*workflow.WorkflowInstance, error) {
	args := m.Called(ctx, workflowID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*workflow.WorkflowInstance), args.Error(1)
}

// ResolveAdminIntervention makes MockWM also satisfy workflow.AdminInterventionResolver —
// needed only by the ResolveAdminIntervention tests, which type-assert s.wm against it the same
// way the real Service.ResolveAdminIntervention does.
func (m *MockWM) ResolveAdminIntervention(ctx context.Context, workflowID, runID string, resolution workflow.AdminResolutionSignal) error {
	args := m.Called(ctx, workflowID, runID, resolution)
	return args.Error(0)
}

// ForceParkActivation makes MockWM also satisfy workflow.ActivationParker, for the force-park tests.
func (m *MockWM) ForceParkActivation(ctx context.Context, workflowID, runID, activationID, reason string) error {
	args := m.Called(ctx, workflowID, runID, activationID, reason)
	return args.Error(0)
}

// MockUserService implements user.Service for testing.

// newTestService is a Service with only taskStore; a test sets the managers and task
// canceller it uses through the register helpers below.
func newTestService(taskStore TaskStore) *Service {
	return &Service{taskStore: taskStore}
}

// RegisterWorkflowManager sets the flow workflow manager, for a test.
func (s *Service) RegisterWorkflowManager(wm workflow.Manager) error {
	s.wm = wm
	return nil
}

// RegisterTaskWorkflowManager sets the task workflow manager, for a test.
func (s *Service) RegisterTaskWorkflowManager(wm workflow.Manager) error {
	s.taskWm = wm
	return nil
}

// RegisterTaskCanceller sets what closes a force-parked node's task, for a test.
func (s *Service) RegisterTaskCanceller(c TaskCanceller) error {
	s.tasks = c
	return nil
}

func withAuthContext(ctx context.Context, userID string) context.Context {
	return authn.ContextWithPrincipal(ctx, &authn.Principal{
		Kind:   authn.KindUser,
		UserID: userID,
		Email:  userID + "@example.com",
	})
}
