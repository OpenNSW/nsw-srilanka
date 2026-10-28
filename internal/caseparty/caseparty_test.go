package caseparty

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

// Assigning a role upserts on (case_id, role), so a second assignment replaces the holder
// rather than adding one.
func TestAssignReplacesTheHolder(t *testing.T) {
	db, mock := newMockDB(t)
	by := "user-1"
	mock.ExpectExec(`INSERT INTO "case_parties" \("case_id","role","party_type","party_id","assigned_by","created_at","updated_at"\) `+
		`VALUES .* ON CONFLICT \("case_id","role"\) DO UPDATE SET "party_type"="excluded"."party_type","party_id"="excluded"."party_id",`+
		`"assigned_by"="excluded"."assigned_by","updated_at"="excluded"."updated_at"`).
		WithArgs("C1", "cha", TypeCompany, "cha-co", by, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := Assign(context.Background(), db, Party{CaseID: "C1", Role: "cha", Type: TypeCompany, ID: "cha-co", AssignedBy: &by})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssignRequiresEveryKey(t *testing.T) {
	db, mock := newMockDB(t)
	full := Party{CaseID: "C1", Role: "cha", Type: TypeCompany, ID: "cha-co"}
	for name, p := range map[string]Party{
		"no case id":    {Role: full.Role, Type: full.Type, ID: full.ID},
		"no role":       {CaseID: full.CaseID, Type: full.Type, ID: full.ID},
		"no party type": {CaseID: full.CaseID, Role: full.Role, ID: full.ID},
		"no party id":   {CaseID: full.CaseID, Role: full.Role, Type: full.Type},
	} {
		if err := Assign(context.Background(), db, p); err == nil {
			t.Errorf("%s: Assign = nil error, want one", name)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRolesOf(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT "role" FROM "case_parties" WHERE case_id = \$1 AND \(party_type = \$2 AND party_id = \$3\)`).
		WithArgs("C1", TypeCompany, "acme").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("trader").AddRow("cha"))

	roles, err := NewRepository(db).RolesOf(context.Background(), "C1", Company("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 || !roles["trader"] || !roles["cha"] {
		t.Errorf("roles = %v, want trader and cha", roles)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// With several parties, a role held by any of them counts.
func TestRolesOfAnyParty(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT "role" FROM "case_parties" WHERE case_id = \$1 AND `+
		`\(\(party_type = \$2 AND party_id = \$3\) OR \(party_type = \$4 AND party_id = \$5\)\)`).
		WithArgs("C1", TypeCompany, "acme", TypeUser, "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("officer"))

	roles, err := NewRepository(db).RolesOf(context.Background(), "C1", Company("acme"), Ref{Type: TypeUser, ID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 || !roles["officer"] {
		t.Errorf("roles = %v, want officer", roles)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A case that does not exist, or that the party is not on, holds no roles: an empty map,
// not an error, so the caller is denied rather than failed.
func TestRolesOfNoMatch(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT "role" FROM "case_parties"`).
		WillReturnRows(sqlmock.NewRows([]string{"role"}))

	roles, err := NewRepository(db).RolesOf(context.Background(), "gone", Company("acme"))
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(roles) != 0 {
		t.Errorf("roles = %v, want none", roles)
	}
}

// A database failure is a real one and must not be flattened into "no roles".
func TestRolesOfPropagatesFailures(t *testing.T) {
	db, mock := newMockDB(t)
	boom := errors.New("connection refused")
	mock.ExpectQuery(`SELECT "role" FROM "case_parties"`).WillReturnError(boom)

	if _, err := NewRepository(db).RolesOf(context.Background(), "C1", Company("acme")); !errors.Is(err, boom) {
		t.Errorf("got %v, want the underlying error", err)
	}
}

// No case or no party: nothing to match, so no query.
func TestRolesOfSkipsTheQuery(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewRepository(db)
	for name, call := range map[string]func() (map[string]bool, error){
		"no case id": func() (map[string]bool, error) { return repo.RolesOf(context.Background(), "", Company("acme")) },
		"no parties": func() (map[string]bool, error) { return repo.RolesOf(context.Background(), "C1") },
	} {
		roles, err := call()
		if err != nil || len(roles) != 0 {
			t.Errorf("%s: roles = %v, err = %v; want none, nil", name, roles, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
