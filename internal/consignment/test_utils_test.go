package consignment

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
)

// MockTaskStore implements consignment.TaskStore for testing.
type MockTaskStore struct {
	mock.Mock
}

func (m *MockTaskStore) GetAllTasks(ctx context.Context, parentWorkflowID string) []store.TaskRecord {
	args := m.Called(ctx, parentWorkflowID)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]store.TaskRecord)
}

// MockWM implements workflow.Manager for testing.
type MockWM struct {
	mock.Mock
}

func (m *MockWM) StartWorkflow(ctx context.Context, ID string, workflowDefinition workflow.WorkflowDefinition, initialWorkflowVariables map[string]any) error {
	args := m.Called(ctx, ID, workflowDefinition, initialWorkflowVariables)
	return args.Error(0)
}

func (m *MockWM) TaskDone(ctx context.Context, workflowID, runID, nodeID string, output map[string]any) error {
	args := m.Called(ctx, workflowID, runID, nodeID, output)
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

// mockLoader is a simple loader for test artifacts
type mockLoader struct {
	content map[string][]byte
}

func (m *mockLoader) Load(ctx context.Context, path string) ([]byte, error) {
	if data, ok := m.content[path]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("artifact not found at path: %s", path)
}

func setupTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	mockDB, sqlMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       mockDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a gorm database connection", err)
	}

	return db, sqlMock
}
