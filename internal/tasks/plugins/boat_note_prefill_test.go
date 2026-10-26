package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugin reads the declaration the retrieve step captured and the dispatch
// notes listed against it, the declarant and exporter codes, and the
// consignment's container rows, and records the prefill under its output
// namespace. It retrieves only the dispatch notes, and a failed retrieve still
// completes the step, with the prefill not reported as retrieved.
func TestBoatNotePrefill_RecordsThePrefill(t *testing.T) {
	var sent []map[string]any
	plugin := &BoatNotePrefillPlugin{post: func(_ context.Context, serviceID, path string, body any) (map[string]any, error) {
		raw, _ := json.Marshal(body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		b["_service"], b["_path"] = serviceID, path
		sent = append(sent, b)
		return nil, errors.New("503 Service Unavailable")
	}}
	ctx := plugins.PluginContext{
		Context: context.Background(),
		Record:  &store.TaskRecord{TaskID: "task-1", Data: map[string]any{}},
		Inputs: map[string]any{
			"declaration": map[string]any{"generalSegment": map[string]any{"transportVesselName": "EVER GIVEN"}},
			"cdn_refs": []any{
				map[string]any{"office": "CBEX1", "year": "2026", "serial": "C", "number": float64(1), "status": "Registered"},
			},
			"userform": map[string]any{"traders": map[string]any{
				"declarant": map[string]any{"code": "D0012345"},
				"exporter":  map[string]any{"code": "900000000001"},
			}},
			"containers": []any{map[string]any{"cdn_number": "CBEX1/2026/C/1"}},
		},
		OutputNamespace: "boat_note_prefill",
	}

	require.NoError(t, plugin.Execute(ctx, json.RawMessage(`{"service_id": "customs-asycuda"}`)))

	require.Len(t, sent, 1, "only the dispatch note is retrieved; the declaration comes from the retrieve step")
	assert.Equal(t, "customs-asycuda", sent[0]["_service"])
	assert.Equal(t, "/api/cdn/v1/retrieve", sent[0]["_path"])
	assert.Equal(t, "D0012345", sent[0]["declarantCode"])
	assert.Equal(t, "900000000001", sent[0]["companyCode"])
	assert.Equal(t, map[string]any{
		"userform": map[string]any{
			"vessel":     "EVER GIVEN",
			"containers": []map[string]any{{"cdn_number": "CBEX1/2026/C/1"}},
		},
		"retrieved": false,
	}, ctx.Record.Data["boat_note_prefill"])
}

func TestBoatNotePrefill_RequiresAServiceAndANamespace(t *testing.T) {
	plugin := &BoatNotePrefillPlugin{}
	rec := &store.TaskRecord{Data: map[string]any{}}

	require.Error(t, plugin.Execute(plugins.PluginContext{Record: rec, OutputNamespace: "x"}, json.RawMessage(`{}`)))
	require.Error(t, plugin.Execute(plugins.PluginContext{Record: rec}, json.RawMessage(`{"service_id": "customs-asycuda"}`)))
}
