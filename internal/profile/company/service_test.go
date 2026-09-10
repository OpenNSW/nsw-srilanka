package company

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
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
	gormDB, err := gorm.Open(dialector, &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}

	return gormDB, mock
}

func setupPingTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mock.ExpectPing() // consumed by gorm.Open's connectivity check
	dialector := postgres.New(postgres.Config{Conn: db})
	gormDB, err := gorm.Open(dialector, &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}

	return gormDB, mock
}

var companyColumns = []string{"id", "name", "ou_handle", "has_cha", "data", "created_at", "updated_at"}

func companyRow(id, name, ouHandle string, hasCHA bool, data []byte) *sqlmock.Rows {
	return sqlmock.NewRows(companyColumns).
		AddRow(id, name, ouHandle, hasCHA, data, now, now)
}

// --- GetCompanyByID ---

func TestService_GetCompanyByID_InvalidID(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.GetCompanyByID(context.Background(), ""); !errors.Is(err, ErrInvalidCompanyID) {
		t.Fatalf("expected ErrInvalidCompanyID, got %v", err)
	}
}

func TestService_GetCompanyByID_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE id = \$1`).
		WithArgs("missing-id", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := svc.GetCompanyByID(context.Background(), "missing-id"); !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected ErrCompanyNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetCompanyByID_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE id = \$1`).
		WithArgs("co-1", 1).
		WillReturnError(errors.New("query failed"))

	if _, err := svc.GetCompanyByID(context.Background(), "co-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetCompanyByID_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE id = \$1`).
		WithArgs("co-1", 1).
		WillReturnRows(companyRow("co-1", "Acme", "acme-handle", false, []byte(`{}`)))

	record, err := svc.GetCompanyByID(context.Background(), "co-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if record == nil || record.ID != "co-1" || record.Name != "Acme" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- GetCompanyByOUHandle ---

func TestService_GetCompanyByOUHandle_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE ou_handle = \$1`).
		WithArgs("missing-handle", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := svc.GetCompanyByOUHandle(context.Background(), "missing-handle"); !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected ErrCompanyNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetCompanyByOUHandle_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE ou_handle = \$1`).
		WithArgs("acme", 1).
		WillReturnError(errors.New("query failed"))

	if _, err := svc.GetCompanyByOUHandle(context.Background(), "acme"); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetCompanyByOUHandle_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE ou_handle = \$1`).
		WithArgs("acme-handle", 1).
		WillReturnRows(companyRow("co-1", "Acme", "acme-handle", false, []byte(`{}`)))

	record, err := svc.GetCompanyByOUHandle(context.Background(), "acme-handle")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if record == nil || record.OUHandle != "acme-handle" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_GetCompanyByOUHandle_Empty(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.GetCompanyByOUHandle(context.Background(), ""); !errors.Is(err, ErrInvalidCompanyID) {
		t.Fatalf("expected ErrInvalidCompanyID, got %v", err)
	}
}

// --- ListCompanies ---

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

func TestService_ListCompanies_NoFilter(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	rows := sqlmock.NewRows(companyColumns).
		AddRow("adam-pvt-ltd", "ADAM PVT LTD", "adam-pvt-ltd", true, []byte(`{}`), now, now).
		AddRow("edward-pvt-ltd", "EDWARD PVT LTD", "edward-pvt-ltd", true, []byte(`{}`), now, now)
	mock.ExpectQuery(`SELECT .* FROM "company_records" ORDER BY name ASC LIMIT \$1`).
		WithArgs(50).
		WillReturnRows(rows)

	result, err := svc.ListCompanies(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 || result.Items[0].ID != "adam-pvt-ltd" || result.Items[1].ID != "edward-pvt-ltd" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ListCompanies_HasCHA(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE has_cha = \$1 ORDER BY name ASC LIMIT \$2`).
		WithArgs(true, 50).
		WillReturnRows(companyRow("adam-pvt-ltd", "ADAM PVT LTD", "adam-pvt-ltd", true, []byte(`{}`)))

	result, err := svc.ListCompanies(context.Background(), ListFilter{HasCHA: boolPtr(true)})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || !result.Items[0].HasCHA {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ListCompanies_Name(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE name ILIKE \$1 ORDER BY name ASC LIMIT \$2`).
		WithArgs("%adam%", 50).
		WillReturnRows(companyRow("adam-pvt-ltd", "ADAM PVT LTD", "adam-pvt-ltd", true, []byte(`{}`)))

	result, err := svc.ListCompanies(context.Background(), ListFilter{Name: strPtr("adam")})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != "adam-pvt-ltd" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ListCompanies_NameWhitespaceIgnored(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	// Whitespace-only name should NOT add a WHERE clause.
	mock.ExpectQuery(`SELECT .* FROM "company_records" ORDER BY name ASC LIMIT \$1`).
		WithArgs(50).
		WillReturnRows(sqlmock.NewRows(companyColumns))

	_, err := svc.ListCompanies(context.Background(), ListFilter{Name: strPtr("   ")})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ListCompanies_Combined(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" WHERE has_cha = \$1 AND name ILIKE \$2 ORDER BY name ASC LIMIT \$3`).
		WithArgs(true, "%adam%", 50).
		WillReturnRows(companyRow("adam-pvt-ltd", "ADAM PVT LTD", "adam-pvt-ltd", true, []byte(`{}`)))

	result, err := svc.ListCompanies(context.Background(), ListFilter{HasCHA: boolPtr(true), Name: strPtr("adam")})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ListCompanies_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectQuery(`SELECT .* FROM "company_records" ORDER BY name ASC LIMIT \$1`).
		WithArgs(50).
		WillReturnError(errors.New("db down"))

	if _, err := svc.ListCompanies(context.Background(), ListFilter{}); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ListCompanies_CountQueryError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	offset := 10
	// Non-zero offset means the optimisation branch is skipped and COUNT is always run.
	mock.ExpectQuery(`SELECT .* FROM "company_records" ORDER BY name ASC LIMIT \$1 OFFSET \$2`).
		WithArgs(50, 10).
		WillReturnRows(sqlmock.NewRows(companyColumns))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "company_records"`).
		WillReturnError(errors.New("count failed"))

	if _, err := svc.ListCompanies(context.Background(), ListFilter{Offset: &offset}); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- UpdateCompany ---

func TestService_UpdateCompany_InvalidID(t *testing.T) {
	svc := NewService(nil)
	if err := svc.UpdateCompany(context.Background(), "", map[string]any{"k": "v"}); !errors.Is(err, ErrInvalidCompanyID) {
		t.Fatalf("expected ErrInvalidCompanyID, got %v", err)
	}
}

func TestService_UpdateCompany_EmptyData(t *testing.T) {
	svc := NewService(nil)
	// Empty data is a no-op — no DB call should be made.
	if err := svc.UpdateCompany(context.Background(), "co-1", map[string]any{}); err != nil {
		t.Fatalf("expected no error for empty data, got %v", err)
	}
}

func TestService_UpdateCompany_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	// Atomic JSONB merge: no prior SELECT, UPDATE returns 0 rows when id is missing.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "missing-id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := svc.UpdateCompany(context.Background(), "missing-id", map[string]any{"k": "v"}); !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected ErrCompanyNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpdateCompany_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "co-1").
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	if err := svc.UpdateCompany(context.Background(), "co-1", map[string]any{"new": "key"}); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpdateCompany_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "co-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := svc.UpdateCompany(context.Background(), "co-1", map[string]any{"new": "key"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- UpdateCompanyFields ---

func TestService_UpdateCompanyFields_InvalidID(t *testing.T) {
	svc := NewService(nil)
	err := svc.UpdateCompanyFields(context.Background(), "", CompanyFieldsUpdate{Name: strPtr("New Name")})
	if !errors.Is(err, ErrInvalidCompanyID) {
		t.Fatalf("expected ErrInvalidCompanyID, got %v", err)
	}
}

func TestService_UpdateCompanyFields_EmptyName(t *testing.T) {
	svc := NewService(nil)
	err := svc.UpdateCompanyFields(context.Background(), "co-1", CompanyFieldsUpdate{Name: strPtr("   ")})
	if err == nil {
		t.Fatal("expected error for blank name, got nil")
	}
}

func TestService_UpdateCompanyFields_EmptyOUHandle(t *testing.T) {
	svc := NewService(nil)
	err := svc.UpdateCompanyFields(context.Background(), "co-1", CompanyFieldsUpdate{OUHandle: strPtr("  ")})
	if err == nil {
		t.Fatal("expected error for blank ou_handle, got nil")
	}
}

func TestService_UpdateCompanyFields_NoFields(t *testing.T) {
	svc := NewService(nil)
	// No fields set is a no-op — no DB call should be made.
	if err := svc.UpdateCompanyFields(context.Background(), "co-1", CompanyFieldsUpdate{}); err != nil {
		t.Fatalf("expected no error for empty update, got %v", err)
	}
}

func TestService_UpdateCompanyFields_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "missing-id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := svc.UpdateCompanyFields(context.Background(), "missing-id", CompanyFieldsUpdate{Name: strPtr("New Name")})
	if !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected ErrCompanyNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpdateCompanyFields_OUHandleConflict(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "co-1").
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "company_records_ou_handle_key"})
	mock.ExpectRollback()

	err := svc.UpdateCompanyFields(context.Background(), "co-1", CompanyFieldsUpdate{OUHandle: strPtr("taken-handle")})
	if !errors.Is(err, ErrOUHandleConflict) {
		t.Fatalf("expected ErrOUHandleConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpdateCompanyFields_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "co-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.UpdateCompanyFields(context.Background(), "co-1", CompanyFieldsUpdate{
		Name:     strPtr("New Name"),
		OUHandle: strPtr("new-handle"),
		HasCHA:   boolPtr(true),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- ReplaceCompanyData ---

func TestService_ReplaceCompanyData_InvalidID(t *testing.T) {
	svc := NewService(nil)
	err := svc.ReplaceCompanyData(context.Background(), "", json.RawMessage(`{"k":"v"}`))
	if !errors.Is(err, ErrInvalidCompanyID) {
		t.Fatalf("expected ErrInvalidCompanyID, got %v", err)
	}
}

func TestService_ReplaceCompanyData_InvalidJSON(t *testing.T) {
	svc := NewService(nil)
	err := svc.ReplaceCompanyData(context.Background(), "co-1", json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestService_ReplaceCompanyData_NonObjectRejected(t *testing.T) {
	svc := NewService(nil)
	for _, data := range []string{`[1,2,3]`, `"a string"`, `42`, `true`, `null`} {
		err := svc.ReplaceCompanyData(context.Background(), "co-1", json.RawMessage(data))
		if err == nil {
			t.Fatalf("expected error for non-object JSON %q, got nil", data)
		}
	}
}

func TestService_ReplaceCompanyData_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "missing-id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := svc.ReplaceCompanyData(context.Background(), "missing-id", json.RawMessage(`{"k":"v"}`))
	if !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected ErrCompanyNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ReplaceCompanyData_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "co-1").
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	err := svc.ReplaceCompanyData(context.Background(), "co-1", json.RawMessage(`{"k":"v"}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ReplaceCompanyData_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "co-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ReplaceCompanyData(context.Background(), "co-1", json.RawMessage(`{"address":{"city":"Colombo"}}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_ReplaceCompanyData_EmptyDefaultsToEmptyObject(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "company_records" SET`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "co-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := svc.ReplaceCompanyData(context.Background(), "co-1", nil); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- UpsertCompany ---

func TestService_UpsertCompany_InvalidID(t *testing.T) {
	svc := NewService(nil)
	err := svc.UpsertCompany(context.Background(), &Record{Name: "ACME"})
	if !errors.Is(err, ErrInvalidCompanyID) {
		t.Fatalf("expected ErrInvalidCompanyID, got %v", err)
	}
}

func TestService_UpsertCompany_EmptyName(t *testing.T) {
	svc := NewService(nil)
	err := svc.UpsertCompany(context.Background(), &Record{ID: "co-1"})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestService_UpsertCompany_DefaultsOUHandleAndData(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "company_records".*ON CONFLICT \("id"\) DO UPDATE`).
		WithArgs("co-1", "ACME", "co-1", false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow([]byte(`{}`)))
	mock.ExpectCommit()

	record := &Record{ID: "co-1", Name: "ACME"}
	if err := svc.UpsertCompany(context.Background(), record); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if record.OUHandle != "co-1" {
		t.Errorf("expected OUHandle to default to ID, got %q", record.OUHandle)
	}
	if string(record.Data) != "{}" {
		t.Errorf("expected Data to default to {}, got %q", record.Data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpsertCompany_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "company_records".*ON CONFLICT \("id"\) DO UPDATE`).
		WithArgs("co-1", "ACME", "acme", true, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow([]byte(`{"br_no":"123"}`)))
	mock.ExpectCommit()

	record := &Record{ID: "co-1", Name: "ACME", OUHandle: "acme", HasCHA: true, Data: json.RawMessage(`{"br_no":"123"}`)}
	if err := svc.UpsertCompany(context.Background(), record); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpsertCompany_OUHandleConflict(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "company_records".*ON CONFLICT \("id"\) DO UPDATE`).
		WithArgs("co-1", "ACME", "taken-handle", true, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "company_records_ou_handle_key"})
	mock.ExpectRollback()

	record := &Record{ID: "co-1", Name: "ACME", OUHandle: "taken-handle", HasCHA: true, Data: json.RawMessage(`{}`)}
	err := svc.UpsertCompany(context.Background(), record)
	if !errors.Is(err, ErrOUHandleConflict) {
		t.Fatalf("expected ErrOUHandleConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_UpsertCompany_DBError(t *testing.T) {
	db, mock := setupTestDB(t)
	svc := NewService(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "company_records".*ON CONFLICT \("id"\) DO UPDATE`).
		WithArgs("co-1", "ACME", "acme", true, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	record := &Record{ID: "co-1", Name: "ACME", OUHandle: "acme", HasCHA: true, Data: json.RawMessage(`{}`)}
	if err := svc.UpsertCompany(context.Background(), record); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Health ---

func TestService_Health_Success(t *testing.T) {
	db, mock := setupPingTestDB(t)
	svc := NewService(db)

	mock.ExpectPing()

	if err := svc.Health(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestService_Health_DBError(t *testing.T) {
	db, mock := setupPingTestDB(t)
	svc := NewService(db)

	mock.ExpectPing().WillReturnError(errors.New("health failed"))

	if err := svc.Health(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
