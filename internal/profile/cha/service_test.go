package cha

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	dialector := postgres.New(postgres.Config{Conn: db})
	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}

	return gormDB, mock
}

var chaColumns = []string{"id", "name", "description", "email", "company_id", "created_at", "updated_at"}

func chaRow(id, name, email string) *sqlmock.Rows {
	return sqlmock.NewRows(chaColumns).
		AddRow(id, name, "", email, "test-company", time.Now(), time.Now())
}

// --- GetByID ---

func TestService_GetByID_InvalidID(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.GetByID(context.TODO(), ""); !errors.Is(err, ErrInvalidCHAID) { //nolint:staticcheck
		t.Fatalf("expected ErrInvalidCHAID, got %v", err)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "customs_house_agents" WHERE id = \$1`).
		WithArgs("missing", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := svc.GetByID(context.TODO(), "missing"); !errors.Is(err, ErrCHANotFound) { //nolint:staticcheck
		t.Fatalf("expected ErrCHANotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetByID_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "customs_house_agents" WHERE id = \$1`).
		WithArgs("cha-1", 1).
		WillReturnRows(chaRow("cha-1", "Advantis", "advantis@example.com"))

	record, err := svc.GetByID(context.TODO(), "cha-1") //nolint:staticcheck
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if record == nil || record.ID != "cha-1" || record.Name != "Advantis" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- GetByEmail ---

func TestService_GetByEmail_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "customs_house_agents" WHERE email = \$1`).
		WithArgs("nobody@example.com", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := svc.GetByEmail(context.TODO(), "nobody@example.com"); !errors.Is(err, ErrCHANotFound) { //nolint:staticcheck
		t.Fatalf("expected ErrCHANotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetByEmail_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "customs_house_agents" WHERE email = \$1`).
		WithArgs("agent@example.com", 1).
		WillReturnRows(chaRow("cha-2", "Yusen", "agent@example.com"))

	record, err := svc.GetByEmail(context.TODO(), "agent@example.com") //nolint:staticcheck
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if record == nil || record.ID != "cha-2" || record.Email != "agent@example.com" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- List ---

func TestService_List_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "customs_house_agents"`).
		WillReturnError(errors.New("query failed"))

	if _, err := svc.List(context.TODO()); err == nil { //nolint:staticcheck
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_List_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "customs_house_agents"`).
		WillReturnRows(
			sqlmock.NewRows(chaColumns).
				AddRow("cha-1", "Advantis", "", "a@example.com", "test-company", time.Now(), time.Now()).
				AddRow("cha-2", "Yusen", "", "y@example.com", "test-company", time.Now(), time.Now()),
		)

	records, err := svc.List(context.TODO()) //nolint:staticcheck
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Health ---

func TestService_Health_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "customs_house_agents"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	if err := svc.Health(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_Health_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "customs_house_agents"`).
		WillReturnError(errors.New("health failed"))

	if err := svc.Health(); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
