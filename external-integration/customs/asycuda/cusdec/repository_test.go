package cusdec

import (
	"context"
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

// The case's declaration is the most recently submitted one Customs registered.
// Ordering by the last update would let a later event on an older declaration
// (paid, warranted) make it the case's declaration again.
func TestGetRegisteredByCase_TakesTheLatestSubmission(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT * FROM "cusdec_declarations" WHERE case_id = $1 AND cusdec_number > 0 ORDER BY created_at DESC,"cusdec_declarations"."id" LIMIT $2`)).
		WithArgs("case-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "case_id", "cusdec_number"}).AddRow("decl-2", "case-1", 69399))

	decl, err := NewDeclarationRepository(db).GetRegisteredByCase(context.Background(), "case-1")

	require.NoError(t, err)
	require.NotNil(t, decl)
	assert.Equal(t, "decl-2", decl.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRegisteredByCase_IsNilWithNoRegisteredDeclaration(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "cusdec_declarations"`).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	decl, err := NewDeclarationRepository(db).GetRegisteredByCase(context.Background(), "case-1")

	require.NoError(t, err)
	assert.Nil(t, decl)
}
