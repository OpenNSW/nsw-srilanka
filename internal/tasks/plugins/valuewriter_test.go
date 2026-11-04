package plugins

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func valueWriterCtx(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Context:         context.Background(),
		Inputs:          inputs,
		Record:          &store.TaskRecord{TaskID: "task-1", Data: map[string]any{}},
		OutputNamespace: "written",
	}
}

func execValueWriter(ctx plugins.PluginContext, props string) error {
	return NewValueWriterPlugin().Execute(ctx, json.RawMessage(props))
}

func written(t *testing.T, ctx plugins.PluginContext) map[string]any {
	t.Helper()
	out, ok := ctx.Record.Data["written"].(map[string]any)
	require.True(t, ok, "expected a document at written, got %T", ctx.Record.Data["written"])
	return out
}

// blendSheets is the SLTB case: one CusDec for the application, whose
// reference every blend sheet carries.
func blendSheets() map[string]any {
	return map[string]any{
		"cusdec": map[string]any{"registration_no": "90000001", "year": "2026"},
		"userform": map[string]any{
			"blendsheet_data": []any{
				map[string]any{"blend_no": "26/1911", "cusdec_number": "CBEX1 2026 E 68735"},
				map[string]any{"blend_no": "26/1912"},
			},
			"cusdec_draft": map[string]any{"identification": map[string]any{"officeCode": "CBEX1"}},
		},
	}
}

const blendSheetWrites = `{"writes": [
	{"each": "/userform/blendsheet_data", "path": "0/cusdec_number",
	 "template": "TNSW {year} E {registration_no}",
	 "params": {"year": "/cusdec/year", "registration_no": "/cusdec/registration_no"}},
	{"path": "/userform/cusdec_draft/identification/registrationNumber", "from": "/cusdec/registration_no"}
]}`

func TestValueWriter_WritesEveryElementAndOneField(t *testing.T) {
	ctx := valueWriterCtx(blendSheets())

	require.NoError(t, execValueWriter(ctx, blendSheetWrites))

	form := written(t, ctx)["userform"].(map[string]any)
	for i, item := range form["blendsheet_data"].([]any) {
		assert.Equal(t, "TNSW 2026 E 90000001", item.(map[string]any)["cusdec_number"], "blend sheet %d", i)
	}
	assert.Equal(t, "26/1911", form["blendsheet_data"].([]any)[0].(map[string]any)["blend_no"], "other fields are carried through")
	identification := form["cusdec_draft"].(map[string]any)["identification"].(map[string]any)
	assert.Equal(t, "90000001", identification["registrationNumber"])
	assert.Equal(t, "CBEX1", identification["officeCode"])
}

func TestValueWriter_LeavesInputsAlone(t *testing.T) {
	inputs := blendSheets()
	ctx := valueWriterCtx(inputs)

	require.NoError(t, execValueWriter(ctx, blendSheetWrites))

	first := inputs["userform"].(map[string]any)["blendsheet_data"].([]any)[0].(map[string]any)
	assert.Equal(t, "CBEX1 2026 E 68735", first["cusdec_number"])
}

func TestValueWriter_FromCopiesAnyValue(t *testing.T) {
	ctx := valueWriterCtx(map[string]any{
		"source": map[string]any{"amount": 1200.5, "tags": []any{"a"}},
		"items":  []any{map[string]any{"own": "x"}},
	})

	require.NoError(t, execValueWriter(ctx, `{"writes": [
		{"path": "/copy/amount", "from": "/source/amount"},
		{"each": "/items", "path": "0/tags", "from": "/source/tags"},
		{"each": "/items", "path": "0/mine", "from": "0/own"}
	]}`))

	out := written(t, ctx)
	assert.Equal(t, 1200.5, out["copy"].(map[string]any)["amount"], "a missing parent object is created")
	item := out["items"].([]any)[0].(map[string]any)
	assert.Equal(t, []any{"a"}, item["tags"])
	assert.Equal(t, "x", item["mine"], "a relative from reads the current element")

	item["tags"].([]any)[0] = "changed"
	assert.Equal(t, "a", out["source"].(map[string]any)["tags"].([]any)[0], "a copied value is its own copy")
}

func TestValueWriter_ReadsEveryValueBeforeWriting(t *testing.T) {
	ctx := valueWriterCtx(map[string]any{"a": "1", "b": "2"})

	require.NoError(t, execValueWriter(ctx, `{"writes": [
		{"path": "/a", "from": "/b"},
		{"path": "/b", "from": "/a"}
	]}`))

	assert.Equal(t, map[string]any{"a": "2", "b": "1"}, written(t, ctx))
}

func TestValueWriter_MissingOrEmptyArrayWritesNothing(t *testing.T) {
	for name, items := range map[string]any{"missing": nil, "empty": []any{}} {
		inputs := map[string]any{"ref": "R1"}
		if items != nil {
			inputs["items"] = items
		}
		ctx := valueWriterCtx(inputs)

		require.NoError(t, execValueWriter(ctx, `{"writes": [{"each": "/items", "path": "0/ref", "from": "/ref"}]}`), name)
		assert.Equal(t, inputs, written(t, ctx), name)
	}
}

func TestValueWriter_MissingSourceFails(t *testing.T) {
	for name, props := range map[string]string{
		"from":           `{"writes": [{"path": "/ref", "from": "/cusdec/registration_no"}]}`,
		"template param": `{"writes": [{"path": "/ref", "template": "TNSW {year}", "params": {"year": "/cusdec/year"}}]}`,
	} {
		ctx := valueWriterCtx(map[string]any{"cusdec": map[string]any{}})

		err := execValueWriter(ctx, props)

		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "has no value", name)
		assert.True(t, plugins.IsPermanent(err), name)
		assert.NotContains(t, ctx.Record.Data, "written", name)
	}
}

func TestValueWriter_Rejected(t *testing.T) {
	for name, tc := range map[string]struct{ props, want string }{
		"no writes":              {`{"writes": []}`, "at least one"},
		"from and template":      {`{"writes": [{"path": "/a", "from": "/b", "template": "x"}]}`, "not both"},
		"neither":                {`{"writes": [{"path": "/a"}]}`, `needs "from" or "template"`},
		"params with from":       {`{"writes": [{"path": "/a", "from": "/b", "params": {"x": "/c"}}]}`, "only fill"},
		"unknown placeholder":    {`{"writes": [{"path": "/a", "template": "{x}-{y}", "params": {"x": "/c"}}]}`, "{y}"},
		"absolute path in each":  {`{"writes": [{"each": "/items", "path": "/a", "from": "/b"}]}`, "must be relative"},
		"relative path, no each": {`{"writes": [{"path": "0/a", "from": "/b"}]}`, "only an entry with"},
		"overlapping paths":      {`{"writes": [{"path": "/a", "from": "/x"}, {"path": "/a/b", "from": "/y"}]}`, "overlaps"},
	} {
		ctx := valueWriterCtx(map[string]any{"b": "1", "c": "2", "x": "3", "y": "4"})

		err := execValueWriter(ctx, tc.props)

		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.want, name)
		assert.True(t, plugins.IsPermanent(err), name)
	}
}

func TestValueWriter_WrongShapesFail(t *testing.T) {
	for name, tc := range map[string]struct {
		inputs map[string]any
		props  string
	}{
		"each is not an array":     {map[string]any{"items": "x", "r": "1"}, `{"writes": [{"each": "/items", "path": "0/a", "from": "/r"}]}`},
		"element is not an object": {map[string]any{"items": []any{"x"}, "r": "1"}, `{"writes": [{"each": "/items", "path": "0/a", "from": "/r"}]}`},
		"param is not a string":    {map[string]any{"n": 1.0}, `{"writes": [{"path": "/a", "template": "{n}", "params": {"n": "/n"}}]}`},
		"write through a value":    {map[string]any{"a": "x", "r": "1"}, `{"writes": [{"path": "/a/b", "from": "/r"}]}`},
	} {
		err := execValueWriter(valueWriterCtx(tc.inputs), tc.props)

		require.Error(t, err, name)
		assert.True(t, plugins.IsPermanent(err), name)
	}
}
