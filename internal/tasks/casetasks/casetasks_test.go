package casetasks

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func mockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

// The listing reads the task table by case alone: no taskflow_task_roles match narrows it.
const tasksOfCaseQuery = `SELECT * FROM "taskflow_tasks" WHERE case_id = $1`

func TestTasksOfCase_ListsEveryTaskOfTheCase(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(tasksOfCaseQuery)).WithArgs("case-1").
		WillReturnRows(sqlmock.NewRows([]string{"task_id", "case_id", "parent_workflow_id", "task_template_id", "state"}).
			AddRow("task-1", "case-1", "wf-1", "review", "COMPLETED").
			AddRow("task-2", "case-1", "wf-2", "payment", "PENDING_USER"))

	got, err := New(db).TasksOfCase(context.Background(), "case-1")

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "task-1", got[0].TaskID)
	assert.Equal(t, "wf-2", got[1].ParentWorkflowID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A failed query is reported, not read as a case with no tasks.
func TestTasksOfCase_ReturnsTheQueryError(t *testing.T) {
	db, mock := mockDB(t)
	boom := errors.New("connection refused")
	mock.ExpectQuery(regexp.QuoteMeta(tasksOfCaseQuery)).WithArgs("case-1").WillReturnError(boom)

	got, err := New(db).TasksOfCase(context.Background(), "case-1")

	assert.ErrorIs(t, err, boom)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}
