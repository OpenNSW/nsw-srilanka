package agency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newMockRepo(t *testing.T) (CaseRepository, sqlmock.Sqlmock) {
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
	return NewCaseRepository(db), mock
}

var officers = Owner{OUHandle: "cda", Roles: []string{"officer"}}

func TestListCasesIsTheOwners(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM cases WHERE EXISTS .*party_type = 'ou'.*party_id = \$1 AND case_parties.role IN \(\$2\)`).
		WithArgs("cda", "officer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, name, state, created_at, updated_at\s+FROM cases\s+WHERE EXISTS .* ORDER BY created_at DESC`).
		WithArgs("cda", "officer", 0, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}).
			AddRow("C1", nil, "IN_PROGRESS", time.Now(), time.Now()))

	cases, total, err := repo.ListCases(context.Background(), officers, 0, 20)
	if err != nil || total != 1 || len(cases) != 1 || cases[0].ID != "C1" {
		t.Fatalf("cases = %+v, total = %d, err = %v", cases, total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// An owner with no OU or no role owns nothing, without a query.
func TestCasesOfNoOwner(t *testing.T) {
	repo, mock := newMockRepo(t)
	for _, o := range []Owner{{Roles: []string{"officer"}}, {OUHandle: "cda"}} {
		if cases, total, err := repo.ListCases(context.Background(), o, 0, 20); err != nil || total != 0 || len(cases) != 0 {
			t.Errorf("%+v: cases = %v, total = %d, err = %v", o, cases, total, err)
		}
		if _, err := repo.GetCase(context.Background(), o, "C1"); !errors.Is(err, ErrCaseNotFound) {
			t.Errorf("%+v: err = %v, want ErrCaseNotFound", o, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetCaseNotOwned(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM cases WHERE id = \$1 AND EXISTS`).
		WithArgs("C1", "cda", "officer").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}))

	if _, err := repo.GetCase(context.Background(), officers, "C1"); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("err = %v, want ErrCaseNotFound", err)
	}
}

// The admin view reads a case whoever owns it.
func TestGetAnyCase(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT id, name, state, created_at, updated_at FROM cases WHERE id = \$1$`).
		WithArgs("C1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}).
			AddRow("C1", nil, "IN_PROGRESS", time.Now(), time.Now()))
	mock.ExpectQuery(`FROM cases WHERE id = \$1$`).
		WithArgs("nope").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "state", "created_at", "updated_at"}))

	if c, err := repo.GetAnyCase(context.Background(), "C1"); err != nil || c.ID != "C1" {
		t.Fatalf("case = %+v, err = %v", c, err)
	}
	if _, err := repo.GetAnyCase(context.Background(), "nope"); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("err = %v, want ErrCaseNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
