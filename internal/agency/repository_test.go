package agency

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newMockRepo(t *testing.T) (Repository, sqlmock.Sqlmock) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: mockDB, DriverName: "postgres"}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(db), mock
}

func TestRecord(t *testing.T) {
	w := Workflow{TaskID: "task-1", TaskCode: "code", CaseID: "C1", CallbackToken: "tok-1"}

	t.Run("new workflow reopens its case", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO agency_workflow`).
			WithArgs("task-1", "code", "C1", StatusStarting, "tok-1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE cases SET state`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		if err := repo.Record(context.Background(), w); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("repeat taskId removes the case it just created", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO agency_workflow`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM cases`).WithArgs("C1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		if err := repo.Record(context.Background(), w); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("repeat taskId keeps a case that already existed", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO agency_workflow`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		if err := repo.Record(context.Background(), w); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}
