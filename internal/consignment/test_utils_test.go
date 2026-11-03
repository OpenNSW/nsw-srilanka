package consignment

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/caseparty"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

// MockTaskStore implements consignment.TaskStore for testing.
type MockTaskStore struct {
	mock.Mock
}

// GetTasks returns the tasks the expectation was set up with, and its error when it was given one.
func (m *MockTaskStore) GetTasks(ctx context.Context, filter store.TaskFilter) ([]store.TaskRecord, error) {
	args := m.Called(ctx, filter)
	tasks, _ := args.Get(0).([]store.TaskRecord)
	if len(args) > 1 {
		return tasks, args.Error(1)
	}
	return tasks, nil
}

type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetUser(ctx context.Context, id string) (*user.Record, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.Record), args.Error(1)
}

func (m *MockUserService) GetOrCreateUser(ctx context.Context, idpUserID, email, phone, ouID, ouHandle string) (string, error) {
	args := m.Called(ctx, idpUserID, email, phone, ouID, ouHandle)
	return args.String(0), args.Error(1)
}

func (m *MockUserService) UpdateUserData(ctx context.Context, id string, data []byte) error {
	return m.Called(ctx, id, data).Error(0)
}

func (m *MockUserService) Health(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

// mustNewService builds a Service with testCatalogRoles, failing the test
// immediately if construction errors.
func mustNewService(t *testing.T, db *gorm.DB, artifactRegistry *artifact.Set, chaService cha.Service, companyService company.Service, userService user.Service, taskStore TaskStore) *Service {
	t.Helper()
	svc, err := NewService(db, artifactRegistry, chaService, companyService, userService, taskStore, testCatalogRoles, caseflow.UserFlows{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func setupTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	mockDB, sqlMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       mockDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a gorm database connection", err)
	}

	return db, sqlMock
}

// consignmentSelect matches Service.withCase's select up to its WHERE: consignments
// joined to their cases row and their trader and CHA company parties.
const consignmentSelect = `SELECT consignments\.id, .* FROM "consignments" JOIN cases ON cases\.id = consignments\.id ` +
	`LEFT JOIN case_parties trader_party ON .* LEFT JOIN case_parties cha_party ON .* WHERE `

// fetchConsignmentQuery matches Service.fetchConsignmentRecord's query; its arguments
// are fetchConsignmentArgs.
const fetchConsignmentQuery = consignmentSelect + `consignments\.id = \$5 ORDER BY "consignments"\."id" LIMIT \$6`

// fetchConsignmentArgs are the arguments of fetchConsignmentQuery for consignment id:
// the party joins' roles and types, then the id and the limit.
func fetchConsignmentArgs(id string) []driver.Value {
	return []driver.Value{partyRoleTrader, caseparty.TypeOU, partyRoleCHA, caseparty.TypeOU, id, 1}
}

// expectCreateConsignment expects createConsignment's transaction: the consignment
// insert, its trader party and its case's creator.
func expectCreateConsignment(sqlMock sqlmock.Sqlmock) {
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec(`(?i)INSERT INTO "consignments"`).WillReturnResult(sqlmock.NewResult(1, 1))
	sqlMock.ExpectExec(`(?i)INSERT INTO "case_parties" .* ON CONFLICT \("case_id","role"\) DO UPDATE`).WillReturnResult(sqlmock.NewResult(1, 1))
	sqlMock.ExpectExec(`UPDATE cases SET created_by = \$1 WHERE id = \$2`).WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectCommit()
}
