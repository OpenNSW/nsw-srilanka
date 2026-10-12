package caseflow

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func newMockRepo(t *testing.T) (Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db), mock
}

func TestEnsureCase(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectExec(`INSERT INTO cases .* ON CONFLICT \(id\) DO NOTHING`).WithArgs("C1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.EnsureCase(context.Background(), "C1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecord(t *testing.T) {
	f := Flow{WorkflowID: "C1:export:u1", CaseID: "C1", TemplateID: "export", Payload: []byte(`{"a":1}`)}

	t.Run("new flow reopens its case", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WithArgs("C1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO case_flows`).
			WithArgs("C1:export:u1", "C1", "export", StatusStarting, []byte(`{"a":1}`)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE cases SET state`).
			WithArgs(CaseInProgress, "C1").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		if err := repo.Record(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("no payload is stored as NULL", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO case_flows`).
			WithArgs("C1:export:u2", "C1", "export", StatusStarting, nil).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE cases SET state`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		if err := repo.Record(context.Background(), Flow{WorkflowID: "C1:export:u2", CaseID: "C1", TemplateID: "export"}); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("repeat workflow ID removes the case it just created", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO case_flows`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM cases`).WithArgs("C1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		if err := repo.Record(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("repeat workflow ID keeps a case that already existed", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO case_flows`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		if err := repo.Record(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a failed insert rolls back", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO cases`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO case_flows`).WillReturnError(sql.ErrConnDone)
		mock.ExpectRollback()

		if err := repo.Record(context.Background(), f); err == nil {
			t.Fatal("want the insert's error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMarkStarted(t *testing.T) {
	t.Run("unknown workflow", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectExec(`UPDATE case_flows`).
			WithArgs(StatusStarting, StatusStarted, "W1").
			WillReturnResult(sqlmock.NewResult(0, 0))

		if err := repo.MarkStarted(context.Background(), "W1"); err != ErrFlowNotFound {
			t.Fatalf("err = %v, want ErrFlowNotFound", err)
		}
	})
}

func TestGet(t *testing.T) {
	columns := []string{"workflow_id", "case_id", "template_id", "status", "payload", "created_at", "updated_at"}
	now := time.Now()

	t.Run("scans the row", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectQuery(`SELECT workflow_id.* FROM case_flows\s+WHERE workflow_id = \$1`).WithArgs("W1").
			WillReturnRows(sqlmock.NewRows(columns).AddRow("W1", "C1", "export", "STARTED", []byte(`{"a":1}`), now, now))

		f, err := repo.Get(context.Background(), "W1")
		if err != nil {
			t.Fatal(err)
		}
		if f.CaseID != "C1" || f.Status != StatusStarted || string(f.Payload) != `{"a":1}` {
			t.Errorf("got %+v", f)
		}
	})

	t.Run("NULL payload", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectQuery(`FROM case_flows`).
			WillReturnRows(sqlmock.NewRows(columns).AddRow("W1", "C1", "export", "STARTING", nil, now, now))

		f, err := repo.Get(context.Background(), "W1")
		if err != nil || f.Payload != nil {
			t.Fatalf("payload = %q, err = %v; want nil, nil", f.Payload, err)
		}
	})

	t.Run("unknown workflow", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectQuery(`FROM case_flows`).WillReturnRows(sqlmock.NewRows(columns))

		if _, err := repo.Get(context.Background(), "W1"); err != ErrFlowNotFound {
			t.Fatalf("err = %v, want ErrFlowNotFound", err)
		}
	})
}

func TestMarkCompleted(t *testing.T) {
	t.Run("locks the case before deciding it is finished", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT case_id FROM case_flows`).WithArgs("W1").
			WillReturnRows(sqlmock.NewRows([]string{"case_id"}).AddRow("C1"))
		mock.ExpectExec(`SELECT id FROM cases WHERE id = \$1 FOR UPDATE`).WithArgs("C1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE case_flows SET status`).WithArgs(StatusCompleted, "W1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE cases SET state`).
			WithArgs(CaseFinished, "C1", StatusCompleted).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		found, err := repo.MarkCompleted(context.Background(), "W1")
		if err != nil || !found {
			t.Fatalf("found = %v, err = %v; want true, nil", found, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unknown workflow", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT case_id FROM case_flows`).WithArgs("W1").
			WillReturnRows(sqlmock.NewRows([]string{"case_id"}))
		mock.ExpectCommit()

		found, err := repo.MarkCompleted(context.Background(), "W1")
		if err != nil || found {
			t.Fatalf("found = %v, err = %v; want false, nil", found, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}
