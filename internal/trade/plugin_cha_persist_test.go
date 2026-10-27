package trade

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

// chaCompanies answers GetCompanyByID with one CHA-enabled company.
type chaCompanies struct {
	company.Service
}

func (chaCompanies) GetCompanyByID(_ context.Context, id string) (*company.Record, error) {
	return &company.Record{ID: id, HasCHA: true}, nil
}

func newCHAPersist(t *testing.T) (*CHAPersistPlugin, sqlmock.Sqlmock) {
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
	return NewCHAPersistPlugin(db, chaCompanies{}), mock
}

// pluginContext is a CHA-selection task of consignment C1, running in a flow whose own
// workflow ID is not the consignment's.
func pluginContext(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Context: context.Background(),
		Record:  &store.TaskRecord{ParentWorkflowID: "C1:aux:u1", CaseID: "C1"},
		Inputs:  inputs,
	}
}

func TestCHAPersistNamesTheCase(t *testing.T) {
	p, mock := newCHAPersist(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "consignments" SET "cha_company_id"=\$1.* WHERE id = \$3`).
		WithArgs("cha-1", sqlmock.AnyArg(), "C1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE cases SET name`).WithArgs("Tea to Rotterdam", "C1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := p.Execute(pluginContext(map[string]any{"cha_company_id": "cha-1", "consignment_name": "Tea to Rotterdam"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCHAPersistWithoutName(t *testing.T) {
	p, mock := newCHAPersist(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "consignments" SET "cha_company_id"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// A blank name leaves the case's name alone.
	err := p.Execute(pluginContext(map[string]any{"cha_company_id": "cha-1", "consignment_name": "  "}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCHAPersistUnknownConsignment(t *testing.T) {
	p, mock := newCHAPersist(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "consignments" SET "cha_company_id"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := p.Execute(pluginContext(map[string]any{"cha_company_id": "cha-1", "consignment_name": "Tea"}), nil)
	if err == nil {
		t.Fatal("want an error for a consignment that does not exist")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCHASeedKeepsOnlyTheDetailsAnAgencyFormReads(t *testing.T) {
	record := &company.Record{
		ID:   "cha-1",
		Name: "CHA ONE PVT LTD",
		Data: json.RawMessage(`{
			"br_no": "PV-1",
			"tin_no": "134256789",
			"tel": "+94112345010",
			"contact_email": "info@cha-one.lk",
			"address": "No. 1, Main Street, Colombo 01",
			"slpacmsuser_key": "secret-slpa-key",
			"agencies": {"cda": {"registration_number": "0002"}}
		}`),
	}

	seed := chaSeed(record)

	assert.Equal(t, map[string]any{
		"id":   "cha-1",
		"name": "CHA ONE PVT LTD",
		"data": map[string]any{
			"tin_no":        "134256789",
			"tel":           "+94112345010",
			"contact_email": "info@cha-one.lk",
			"address":       "No. 1, Main Street, Colombo 01",
		},
	}, seed)
}

func TestCHASeedLeavesOutMissingAndBlankDetails(t *testing.T) {
	seed := chaSeed(&company.Record{ID: "cha-1", Name: "CHA ONE", Data: json.RawMessage(`{"tin_no": "", "tel": "0112345010"}`)})

	assert.Equal(t, map[string]any{"tel": "0112345010"}, seed["data"])
}

func TestCHASeedToleratesUnreadableData(t *testing.T) {
	for _, data := range []json.RawMessage{nil, json.RawMessage(`not json`)} {
		seed := chaSeed(&company.Record{ID: "cha-1", Name: "CHA ONE", Data: data})

		assert.Equal(t, "cha-1", seed["id"])
		assert.Empty(t, seed["data"])
	}
}

func TestCHAPersistEmitsTheChaDetails(t *testing.T) {
	p, mock := newCHAPersist(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "consignments" SET "cha_company_id"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ctx := pluginContext(map[string]any{"cha_company_id": "cha-1"})
	require.NoError(t, p.Execute(ctx, nil))

	cha, ok := ctx.Record.Data["cha"].(map[string]any)
	require.True(t, ok, "the task should output the CHA's details")
	assert.Equal(t, "cha-1", cha["id"])
}

func TestCHAPersistEmitsNothingWhenTheWriteFails(t *testing.T) {
	p, mock := newCHAPersist(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "consignments" SET "cha_company_id"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	ctx := pluginContext(map[string]any{"cha_company_id": "cha-1"})
	require.Error(t, p.Execute(ctx, nil))

	assert.NotContains(t, ctx.Record.Data, "cha")
}
