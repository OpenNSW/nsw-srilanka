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

// chaCompanies answers GetCompanyByID with one CHA-enabled company, whose data holds a TIN and
// an SLPA CMS key.
type chaCompanies struct {
	company.Service
}

func (chaCompanies) GetCompanyByID(_ context.Context, id string) (*company.Record, error) {
	return &company.Record{ID: id, HasCHA: true,
		Data: json.RawMessage(`{"tin_no": "134256789", "slpacmsuser_key": "secret-slpa-key"}`)}, nil
}

// formFields are the data fields the agency forms read, as tnsw/trade/1b-persist_cha lists them.
var formFields = []string{"tin_no", "tel", "contact_email", "address"}

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

// expectCHAParty expects the plugin to record cha-1 as case C1's "cha" party, replacing
// an earlier one.
func expectCHAParty(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`INSERT INTO "case_parties" .* ON CONFLICT \("case_id","role"\) DO UPDATE`).
		WithArgs("C1", "cha", "company", "cha-1", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestCHAPersistNamesTheCase(t *testing.T) {
	p, mock := newCHAPersist(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "consignments" SET "cha_company_id"=\$1.* WHERE id = \$3`).
		WithArgs("cha-1", sqlmock.AnyArg(), "C1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectCHAParty(mock)
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
	expectCHAParty(mock)
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

	seed, err := chaSeed(record, formFields)
	require.NoError(t, err)

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

func TestCHASeedCopiesOnlyTheListedFields(t *testing.T) {
	record := &company.Record{ID: "cha-1", Name: "CHA ONE", Data: json.RawMessage(
		`{"tin_no": "134256789", "vat_no": "VAT-1", "agencies": {"cda": {}}, "slpacmsuser_key": "secret"}`)}

	t.Run("none listed", func(t *testing.T) {
		seed, err := chaSeed(record, nil)
		require.NoError(t, err)

		assert.Equal(t, map[string]any{"id": "cha-1", "name": "CHA ONE", "data": map[string]any{}}, seed)
	})
	t.Run("a field a flow added", func(t *testing.T) {
		seed, err := chaSeed(record, []string{"tin_no", "vat_no"})
		require.NoError(t, err)

		assert.Equal(t, map[string]any{"tin_no": "134256789", "vat_no": "VAT-1"}, seed["data"])
	})
	t.Run("a field that isn't a string", func(t *testing.T) {
		seed, err := chaSeed(record, []string{"agencies"})
		require.NoError(t, err)

		assert.Empty(t, seed["data"])
	})
}

func TestCHASeedLeavesOutMissingAndBlankDetails(t *testing.T) {
	seed, err := chaSeed(&company.Record{ID: "cha-1", Name: "CHA ONE", Data: json.RawMessage(
		`{"tin_no": "", "address": "   ", "contact_email": "\t\n", "tel": "0112345010"}`)}, formFields)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"tel": "0112345010"}, seed["data"])
}

func TestCHASeedWithoutData(t *testing.T) {
	cases := map[string]json.RawMessage{"no data": nil, "empty object": json.RawMessage(`{}`)}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			seed, err := chaSeed(&company.Record{ID: "cha-1", Name: "CHA ONE", Data: data}, formFields)
			require.NoError(t, err)

			assert.Equal(t, "cha-1", seed["id"])
			assert.Empty(t, seed["data"])
		})
	}
}

func TestCHASeedRejectsUnreadableData(t *testing.T) {
	for _, data := range []json.RawMessage{
		json.RawMessage(`not json`), json.RawMessage(`[]`), json.RawMessage(`"x"`), json.RawMessage(`null`),
	} {
		_, err := chaSeed(&company.Record{ID: "cha-1", Name: "CHA ONE", Data: data}, formFields)

		assert.Error(t, err, "data %q", data)
	}
}

// unreadableCHA answers GetCompanyByID with a CHA-enabled company whose data isn't an object.
type unreadableCHA struct {
	company.Service
}

func (unreadableCHA) GetCompanyByID(_ context.Context, id string) (*company.Record, error) {
	return &company.Record{ID: id, HasCHA: true, Data: json.RawMessage(`[]`)}, nil
}

func TestCHAPersistFailsOnUnreadableDataBeforeWriting(t *testing.T) {
	p, mock := newCHAPersist(t)
	p.companyService = unreadableCHA{}

	ctx := pluginContext(map[string]any{"cha_company_id": "cha-1", "consignment_name": "Tea to Rotterdam"})
	err := p.Execute(ctx, nil)

	// sqlmock expects no SQL here, so starting a write would have failed with a different error.
	require.ErrorContains(t, err, `cha_persist: CHA company "cha-1": data is not a JSON object`)
	assert.NotContains(t, ctx.Record.Data, "cha")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCHAPersistEmitsTheConfiguredDetails(t *testing.T) {
	cases := map[string]struct {
		config json.RawMessage
		data   map[string]any
	}{
		"fields listed":    {json.RawMessage(`{"cha_data_fields": ["tin_no", "tel"]}`), map[string]any{"tin_no": "134256789"}},
		"no config":        {nil, map[string]any{}},
		"empty config":     {json.RawMessage(`{}`), map[string]any{}},
		"null config":      {json.RawMessage(`null`), map[string]any{}},
		"empty field list": {json.RawMessage(`{"cha_data_fields": []}`), map[string]any{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, mock := newCHAPersist(t)
			mock.ExpectBegin()
			mock.ExpectExec(`UPDATE "consignments" SET "cha_company_id"`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			expectCHAParty(mock)
			mock.ExpectCommit()

			ctx := pluginContext(map[string]any{"cha_company_id": "cha-1"})
			require.NoError(t, p.Execute(ctx, tc.config))

			cha, ok := ctx.Record.Data["cha"].(map[string]any)
			require.True(t, ok, "the task should output the CHA's details")
			assert.Equal(t, "cha-1", cha["id"])
			assert.Equal(t, tc.data, cha["data"])
		})
	}
}

func TestCHAPersistRejectsAnInvalidConfigBeforeAnything(t *testing.T) {
	for _, config := range []json.RawMessage{
		json.RawMessage(`{"cha_data_fields": "tin_no"}`),
		json.RawMessage(`{"cha_data_fields": [1]}`),
		json.RawMessage(`{"cha_data_fields": ["tin_no", " "]}`),
		json.RawMessage(`[]`),
	} {
		p, mock := newCHAPersist(t)

		ctx := pluginContext(map[string]any{"cha_company_id": "cha-1"})
		err := p.Execute(ctx, config)

		require.ErrorContains(t, err, "cha_persist: invalid plugin config", "config %s", config)
		assert.NotContains(t, ctx.Record.Data, "cha")
		require.NoError(t, mock.ExpectationsWereMet())
	}
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
