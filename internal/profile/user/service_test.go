package user

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var now = time.Now()

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

// --- GetUser ---

func TestService_GetUser_InvalidID(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.GetUser(""); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}

func TestService_GetUser_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE id = \$1`).
		WithArgs("missing-id", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := svc.GetUser("missing-id"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetUser_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	userID := "user-123"
	idpUserID := "idp-123"
	rows := sqlmock.NewRows([]string{"id", "idp_user_id", "email", "phone_number", "ou_id", "ou_handle", "data", "created_at", "updated_at"}).
		AddRow(userID, idpUserID, "user@example.com", "+61400111222", "OU-001", "ou-001", []byte(`{}`), now, now)

	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE id = \$1`).
		WithArgs(userID, 1).
		WillReturnRows(rows)

	record, err := svc.GetUser(userID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if record == nil || record.ID != userID || record.IDPUserID != idpUserID {
		t.Fatalf("unexpected record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- GetOrCreateUser ---

func TestService_GetOrCreateUser_InvalidID(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.GetOrCreateUser("", "user@example.com", "+61400111222", "OU-001", "ou-001"); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}

func TestService_GetOrCreateUser_AlreadyExists(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	rows := sqlmock.NewRows([]string{"id", "idp_user_id", "email", "phone_number", "ou_id", "ou_handle", "data", "created_at", "updated_at"}).
		AddRow("user-123", "idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001", []byte(`{}`), now, now)
	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE idp_user_id = \$1`).
		WithArgs("idp-123", 1).
		WillReturnRows(rows)

	userID, err := svc.GetOrCreateUser("idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if userID == nil || *userID != "user-123" {
		t.Fatalf("expected existing user id, got %v", userID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetOrCreateUser_ExistsCheckDBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE idp_user_id = \$1`).
		WithArgs("idp-123", 1).
		WillReturnError(errors.New("query failed"))

	_, err := svc.GetOrCreateUser("idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001")
	if err == nil || !strings.Contains(err.Error(), "database query failed") {
		t.Fatalf("expected wrapped DB error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetOrCreateUser_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	idpUserID := "idp-123"
	email := "user@example.com"
	phone := "+61400111222"
	ouID := "OU-001"
	ouHandle := "ou-001"

	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE idp_user_id = \$1`).
		WithArgs(idpUserID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "user_records"`).
		WithArgs(sqlmock.AnyArg(), idpUserID, email, phone, ouID, ouHandle, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	userID, err := svc.GetOrCreateUser(idpUserID, email, phone, ouID, ouHandle)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if userID == nil || *userID == "" {
		t.Fatalf("expected created user id, got %v", userID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetOrCreateUser_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE idp_user_id = \$1`).
		WithArgs("idp-123", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "user_records"`).
		WithArgs(sqlmock.AnyArg(), "idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	if _, err := svc.GetOrCreateUser("idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001"); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetOrCreateUser_DuplicateInsertReturnsExisting(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE idp_user_id = \$1`).
		WithArgs("idp-123", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "user_records"`).
		WithArgs(sqlmock.AnyArg(), "idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 0))
	mock.ExpectCommit()

	rows := sqlmock.NewRows([]string{"id", "idp_user_id", "email", "phone_number", "ou_id", "ou_handle", "data", "created_at", "updated_at"}).
		AddRow("user-123", "idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001", []byte(`{}`), now, now)
	mock.ExpectQuery(`SELECT .* FROM "user_records" WHERE idp_user_id = \$1`).
		WithArgs("idp-123", 1).
		WillReturnRows(rows)

	userID, err := svc.GetOrCreateUser("idp-123", "user@example.com", "+61400111222", "OU-001", "ou-001")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if userID == nil || *userID != "user-123" {
		t.Fatalf("expected existing user id, got %v", userID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- UpdateUserData ---

func TestService_UpdateUserData_InvalidID(t *testing.T) {
	svc := NewService(nil)
	if err := svc.UpdateUserData("", []byte(`{"k":"v"}`)); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}

func TestService_UpdateUserData_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "user_records" SET`).
		WithArgs([]byte(`{"k":"v"}`), sqlmock.AnyArg(), "missing-id"). // data, updated_at, id
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := svc.UpdateUserData("missing-id", []byte(`{"k":"v"}`)); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpdateUserData_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "user_records" SET`).
		WithArgs([]byte(`{"k":"v"}`), sqlmock.AnyArg(), "user-123"). // data, updated_at, id
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	if err := svc.UpdateUserData("user-123", []byte(`{"k":"v"}`)); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpdateUserData_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "user_records" SET`).
		WithArgs([]byte(`{"k":"v"}`), sqlmock.AnyArg(), "user-123"). // data, updated_at, id
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := svc.UpdateUserData("user-123", []byte(`{"k":"v"}`)); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Health ---

func TestService_Health_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "user_records"`).WillReturnRows(rows)

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

	mock.ExpectQuery(`SELECT count\(\*\) FROM "user_records"`).WillReturnError(errors.New("health failed"))

	if err := svc.Health(); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
