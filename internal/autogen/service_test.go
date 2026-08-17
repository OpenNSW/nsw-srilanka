package autogen

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	dialector := postgres.New(postgres.Config{
		Conn:       mockDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	return db, mock
}

func setupTestRegistry(t *testing.T) *Registry {
	reg := NewRegistry()
	err := reg.LoadFromYAML([]byte(sampleYAML))
	require.NoError(t, err)
	return reg
}

func TestGenerateID_RTA_ApplicationID_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	reg := setupTestRegistry(t)
	svc := NewSequenceService(db, reg)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "reference_sequences" WHERE scope_key = \$1 ORDER BY "reference_sequences"\."scope_key" LIMIT \$2 FOR UPDATE`).
		WithArgs(sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"scope_key", "current_value", "updated_at"}).
			AddRow("RTA:application_id:COL:"+time.Now().UTC().Format("20060102"), 41, time.Now()))

	mock.ExpectExec(`UPDATE "reference_sequences" SET`).
		WithArgs(int64(42), sqlmock.AnyArg(), "RTA:application_id:COL:"+time.Now().UTC().Format("20060102")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	params := map[string]string{"officeCode": "COL"}
	ref, err := svc.GenerateID(context.Background(), "RTA", "application_id", params)
	require.NoError(t, err)

	expectedPrefix := "RTA-APP-COL-" + time.Now().UTC().Format("20060102") + "-000042"
	assert.Equal(t, expectedPrefix, ref)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateID_RTA_PermitID_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	reg := setupTestRegistry(t)
	svc := NewSequenceService(db, reg)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "reference_sequences" WHERE scope_key = \$1 ORDER BY "reference_sequences"\."scope_key" LIMIT \$2 FOR UPDATE`).
		WithArgs("RTA:permit_id:COL", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectExec(`UPDATE "reference_sequences" SET`).
		WithArgs(int64(1), sqlmock.AnyArg(), "RTA:permit_id:COL").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	params := map[string]string{"officeCode": "COL"}
	ref, err := svc.GenerateID(context.Background(), "RTA", "permit_id", params)
	require.NoError(t, err)

	assert.Equal(t, "RTA-PMT-COL-00000001", ref)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateID_InvalidListParam_Fails(t *testing.T) {
	db, _ := setupTestDB(t)
	reg := setupTestRegistry(t)
	svc := NewSequenceService(db, reg)

	params := map[string]string{"officeCode": "INVALID"}
	_, err := svc.GenerateID(context.Background(), "RTA", "application_id", params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not valid for list")
}

func TestGetOrGenerateID_ExistingFound(t *testing.T) {
	db, mock := setupTestDB(t)
	reg := setupTestRegistry(t)
	svc := NewSequenceService(db, reg)

	mock.ExpectQuery(`SELECT data->>.*FROM "task_records_v2"`).
		WillReturnRows(sqlmock.NewRows([]string{"data->>reference_number"}).AddRow("RTA-APP-COL-20260817-000042"))

	ref, err := svc.GetOrGenerateID(context.Background(), "wf-100", "", "RTA", "application_id", "reference_number", map[string]string{"officeCode": "COL"})
	require.NoError(t, err)
	assert.Equal(t, "RTA-APP-COL-20260817-000042", ref)
	require.NoError(t, mock.ExpectationsWereMet())
}
