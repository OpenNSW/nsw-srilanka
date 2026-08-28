package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSLPAGatePassSplitBuilder_OneBranchPerContainer(t *testing.T) {
	ctx := splitCtx(map[string]any{
		// The shape a task record holds after a round trip through JSON.
		"containers": []any{"CON-FCL-001", "CON-FCL-002"},
		"slug":       "8d326f3a-643a-4a1d-8072-87130288b032",
		"client_key": "agztNvLSUA",
	})
	require.NoError(t, SLPAGatePassSplitBuilderFunc(ctx, nil))

	items, ok := ctx.Record.Data["split_items"].([]map[string]any)
	require.True(t, ok, "expected split_items, got %T", ctx.Record.Data["split_items"])
	require.Len(t, items, 2)

	// A child workflow inherits nothing but its payload, so each branch has to
	// carry the container, the order it belongs to and the company key.
	assert.Equal(t, "gatepass", items[0]["branch_id"])
	assert.Equal(t, map[string]any{
		"container_no":    "CON-FCL-001",
		"slug":            "8d326f3a-643a-4a1d-8072-87130288b032",
		"client_key":      "agztNvLSUA",
		"container_label": "Container CON-FCL-001 (1 of 2)",
	}, items[0]["payload"])
	assert.Equal(t, "CON-FCL-002", items[1]["payload"].(map[string]any)["container_no"])
}

// A pass is issued for what the trader ticked on the consolidation form, and for
// what SLPA had already paired before they got there.
func TestSLPAGatePassSplitBuilder_TakesTheTradersSelection(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"containers": []any{"CON-ALREADY"}, // already consolidated by SLPA
		"rows": []any{
			map[string]any{"cap_container_no": "CON-TICKED", "consolidate": true},
			map[string]any{"cap_container_no": "CON-DECLINED", "consolidate": false},
		},
	})
	require.NoError(t, SLPAGatePassSplitBuilderFunc(ctx, nil))

	items := ctx.Record.Data["split_items"].([]map[string]any)
	var got []string
	for _, item := range items {
		got = append(got, item["payload"].(map[string]any)["container_no"].(string))
	}
	assert.Equal(t, []string{"CON-ALREADY", "CON-TICKED"}, got,
		"a container the trader declined is not consolidated, so it gets no pass")
}

// SLPA issues one pass per container, so a container listed twice must not spawn
// two branches for it.
func TestSLPAGatePassSplitBuilder_DeduplicatesContainers(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"containers": []any{"CON-A", " con-a ", "CON-B", ""},
	})
	require.NoError(t, SLPAGatePassSplitBuilderFunc(ctx, nil))

	items := ctx.Record.Data["split_items"].([]map[string]any)
	require.Len(t, items, 2)
	assert.Equal(t, "CON-A", items[0]["payload"].(map[string]any)["container_no"])
	assert.Equal(t, "CON-B", items[1]["payload"].(map[string]any)["container_no"])
}

// The task node and the subtask node are two different mappings, so a value can
// be on the record without being among the step's inputs. Failing on which one
// named it would strand a consignment whose data is right there.
func TestSLPAGatePassSplitBuilder_ReadsTheRecordWhenInputsAreBare(t *testing.T) {
	ctx := splitCtx(map[string]any{})
	ctx.Record.Data["rows"] = []any{
		map[string]any{"cap_container_no": "MSCU8492019", "consolidate": true},
	}
	ctx.Record.Data["slug"] = "88ec481f-726c-4eb3-971c-1b280a3e7aec"
	require.NoError(t, SLPAGatePassSplitBuilderFunc(ctx, nil))

	items := ctx.Record.Data["split_items"].([]map[string]any)
	require.Len(t, items, 1)
	payload := items[0]["payload"].(map[string]any)
	assert.Equal(t, "MSCU8492019", payload["container_no"])
	assert.Equal(t, "88ec481f-726c-4eb3-971c-1b280a3e7aec", payload["slug"])
}

// A fan-out over nothing completes silently, which would leave the consignment
// with no gate pass and no explanation of why.
func TestSLPAGatePassSplitBuilder_RefusesAnEmptyFanOut(t *testing.T) {
	for name, inputs := range map[string]map[string]any{
		"neither source":  {},
		"empty list":      {"containers": []any{}},
		"blank entries":   {"containers": []any{"", "  "}},
		"not a list":      {"containers": "CON-A"},
		"nothing ticked":  {"rows": []any{map[string]any{"cap_container_no": "CON-A", "consolidate": false}}},
		"rows not a list": {"rows": "CON-A"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := splitCtx(inputs)
			require.Error(t, SLPAGatePassSplitBuilderFunc(ctx, nil))
			assert.NotContains(t, ctx.Record.Data, "split_items")
		})
	}
}
