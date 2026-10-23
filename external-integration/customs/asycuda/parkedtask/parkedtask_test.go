package parkedtask

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type tasksOfCase map[string][]store.TaskRecord

func (t tasksOfCase) TasksOfCase(_ context.Context, caseID string) ([]store.TaskRecord, error) {
	return t[caseID], nil
}

type failingTasks struct{ err error }

func (f failingTasks) TasksOfCase(context.Context, string) ([]store.TaskRecord, error) {
	return nil, f.err
}

func TestFind(t *testing.T) {
	tasks := tasksOfCase{"case-1": {
		{TaskID: "done", ActiveStepID: "s0", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "wait", State: "COMPLETED"},
		{TaskID: "other-wf", ActiveStepID: "s1", ParentWorkflowID: "wf-2", ActiveTaskTemplateID: "wait", State: StateQueuedExternally},
		{TaskID: "other-template", ActiveStepID: "s2", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "review", State: StateQueuedExternally},
		{TaskID: "parked", ActiveStepID: "s3", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "wait", State: StateQueuedExternally},
	}}

	got, ok, err := Find(context.Background(), tasks, "case-1", "wf-1", "wait")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, Task{TaskID: "parked", StepID: "s3"}, got)

	for name, args := range map[string][3]string{
		"another case":         {"case-2", "wf-1", "wait"},
		"no workflow recorded": {"case-1", "", "wait"},
		"no case recorded":     {"", "wf-1", "wait"},
		"nothing parked on it": {"case-1", "wf-1", "ack"},
	} {
		_, ok, err := Find(context.Background(), tasks, args[0], args[1], args[2])
		require.NoError(t, err, name)
		assert.False(t, ok, name)
	}
}

// A load that fails is reported, not read as "nothing is parked".
func TestFind_ReportsALoadThatFailed(t *testing.T) {
	boom := errors.New("connection refused")

	_, ok, err := Find(context.Background(), failingTasks{err: boom}, "case-1", "wf-1", "wait")

	assert.ErrorIs(t, err, boom)
	assert.False(t, ok)
}

func mockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

const tasksOfCaseQuery = `SELECT * FROM "task_records_v2" WHERE case_id = $1`

// The store returns a failed query's error rather than an empty list.
func TestStore_ReturnsTheQueryError(t *testing.T) {
	db, mock := mockDB(t)
	boom := errors.New("connection refused")
	mock.ExpectQuery(regexp.QuoteMeta(tasksOfCaseQuery)).WithArgs("case-1").WillReturnError(boom)

	got, err := NewStore(db).TasksOfCase(context.Background(), "case-1")

	assert.ErrorIs(t, err, boom)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_ReadsTheTasksOfTheCase(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(tasksOfCaseQuery)).WithArgs("case-1").
		WillReturnRows(sqlmock.NewRows([]string{"task_id", "case_id", "parent_workflow_id", "active_task_template_id", "active_step_id", "state"}).
			AddRow("task-1", "case-1", "wf-1", "wait", "step-1", StateQueuedExternally))

	got, err := NewStore(db).TasksOfCase(context.Background(), "case-1")

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "task-1", got[0].TaskID)
	assert.Equal(t, "wf-1", got[0].ParentWorkflowID)
	assert.Equal(t, "step-1", got[0].ActiveStepID)
	assert.Equal(t, StateQueuedExternally, got[0].State)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// FindInCase finds a parked task in any workflow of the case: a wait in a
// branch of a parallel split has a workflow of its own.
func TestFindInCase(t *testing.T) {
	tasks := tasksOfCase{"case-1": {
		{TaskID: "done", ActiveStepID: "s0", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "release", State: "COMPLETED"},
		{TaskID: "other-template", ActiveStepID: "s1", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "review", State: StateQueuedExternally},
		{TaskID: "in-branch", ActiveStepID: "s2", ParentWorkflowID: "wf-branch", ActiveTaskTemplateID: "release", State: StateQueuedExternally},
	}}

	got, ok, err := FindInCase(context.Background(), tasks, "case-1", "release")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, Task{TaskID: "in-branch", StepID: "s2"}, got)

	for name, args := range map[string][2]string{
		"another case":         {"case-2", "release"},
		"no case recorded":     {"", "release"},
		"nothing parked on it": {"case-1", "ack"},
	} {
		_, ok, err := FindInCase(context.Background(), tasks, args[0], args[1])
		require.NoError(t, err, name)
		assert.False(t, ok, name)
	}

	_, _, err = FindInCase(context.Background(), failingTasks{err: errors.New("down")}, "case-1", "release")
	assert.Error(t, err)
}
