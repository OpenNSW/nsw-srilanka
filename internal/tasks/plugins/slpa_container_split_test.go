package plugins

import (
	"fmt"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// containerOrder is the shape the service order's container rows arrive in: the
// trader's own form, so camelCase and every value decoded from JSON.
func containerOrder() []any {
	return []any{
		map[string]any{"containerNo": "DUMY0000001", "containerSize": "40"},
		map[string]any{"containerNo": "DUMY0000002", "containerSize": float64(20)},
	}
}

// One branch per container the trader put on the order, each carrying everything
// its calls need: a child workflow inherits nothing else.
func TestSLPAContainerSplitBuilder_OneBranchPerOrderedContainer(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"containers":    containerOrder(),
		"cusdec_serial": "CBEX1/2026/E/1047",
		"slug":          "8d326f3a-643a-4a1d-8072-87130288b032",
		"client_key":    "agztNvLSUA",
	})
	require.NoError(t, SLPAContainerSplitBuilderFunc(ctx, nil))

	items, ok := ctx.Record.Data["split_items"].([]map[string]any)
	require.True(t, ok, "expected split_items, got %T", ctx.Record.Data["split_items"])
	require.Len(t, items, 2)

	// SAME_TEMPLATE mode suffixes branch_id with the item index, so a constant
	// here still yields unique branches.
	assert.Equal(t, "container", items[0]["branch_id"])

	assert.Equal(t, map[string]any{
		"so_container_no":  "DUMY0000001",
		"cap_container_no": "",
		"deleted":          false,
		"container_size":   "40",
		"cusdec_serial":    "CBEX1/2026/E/1047",
		"slug":             "8d326f3a-643a-4a1d-8072-87130288b032",
		"client_key":       "agztNvLSUA",
		"container_label":  "Container DUMY0000001 (1 of 2)",
	}, items[0]["payload"])

	second := items[1]["payload"].(map[string]any)
	assert.Equal(t, "DUMY0000002", second["so_container_no"])
	// A coded size arrives as a number and must reach the CMS as its digits.
	assert.Equal(t, "20", second["container_size"])
	assert.Equal(t, "Container DUMY0000002 (2 of 2)", second["container_label"])
}

// The placeholder is the fixed side of the pairing and the branch owns it
// outright, which is what keeps two branches from claiming the same one. The
// real container is seeded empty because it does not exist yet: a condition on a
// variable the child has never been given cannot compile, which parks the branch
// instead of routing it.
func TestSLPAContainerSplitBuilder_SeedsEveryGatewayAValue(t *testing.T) {
	ctx := splitCtx(map[string]any{"containers": containerOrder()})
	require.NoError(t, SLPAContainerSplitBuilderFunc(ctx, nil))

	for _, item := range ctx.Record.Data["split_items"].([]map[string]any) {
		payload := item["payload"].(map[string]any)
		require.Contains(t, payload, "cap_container_no")
		require.Contains(t, payload, "deleted")
		assert.Equal(t, "", payload["cap_container_no"], "no real container is chosen yet")
		assert.Equal(t, false, payload["deleted"])
	}
}

// A row the order accepted but this cannot name is still findable by position,
// rather than becoming a nameless task on the trader's dashboard.
func TestSLPAContainerSplitBuilder_NamesABranchWithNoPlaceholderByPosition(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"containers": []any{
			map[string]any{"containerNo": "  ", "containerSize": "40"},
			map[string]any{"containerNo": "DUMY0000002"},
		},
	})
	require.NoError(t, SLPAContainerSplitBuilderFunc(ctx, nil))

	items := ctx.Record.Data["split_items"].([]map[string]any)
	require.Len(t, items, 2)
	assert.Equal(t, "Container 1 of 2", items[0]["payload"].(map[string]any)["container_label"])
	assert.Equal(t, "Container DUMY0000002 (2 of 2)", items[1]["payload"].(map[string]any)["container_label"])
}

// Both shapes occur: the []any a task record holds after a round trip through
// JSON, and the typed slice a step recorded in Go and handed straight on.
func TestSLPAContainerSplitBuilder_ReadsBothShapesTheRowsArriveIn(t *testing.T) {
	typed := splitCtx(map[string]any{
		"containers": []map[string]any{{"containerNo": "DUMY0000001", "containerSize": "40"}},
	})
	require.NoError(t, SLPAContainerSplitBuilderFunc(typed, nil))

	items := typed.Record.Data["split_items"].([]map[string]any)
	require.Len(t, items, 1)
	assert.Equal(t, "DUMY0000001", items[0]["payload"].(map[string]any)["so_container_no"])
}

// The task node and the subtask node are two different mappings, so a value can
// be on the record without being among the step's inputs. Failing on which one
// named it would strand an order whose data is right there.
func TestSLPAContainerSplitBuilder_ReadsTheRecordWhenInputsAreBare(t *testing.T) {
	ctx := splitCtx(map[string]any{})
	ctx.Record.Data["containers"] = containerOrder()
	ctx.Record.Data["cusdec_serial"] = "CBEX1/2026/E/1047"
	require.NoError(t, SLPAContainerSplitBuilderFunc(ctx, nil))

	items := ctx.Record.Data["split_items"].([]map[string]any)
	require.Len(t, items, 2)
	assert.Equal(t, "CBEX1/2026/E/1047", items[0]["payload"].(map[string]any)["cusdec_serial"])
}

// A SPLIT_TASK over zero items completes silently, which would leave the
// consignment with nothing consolidated, no gate pass, and no account of why.
func TestSLPAContainerSplitBuilder_RefusesAnEmptyOrUnreadableFanOut(t *testing.T) {
	tooMany := make([]any, maxContainersPerOrder+1)
	for i := range tooMany {
		tooMany[i] = map[string]any{"containerNo": fmt.Sprintf("DUMY%07d", i)}
	}

	for name, inputs := range map[string]map[string]any{
		"no containers at all":      {},
		"empty list":                {"containers": []any{}},
		"entries that are not rows": {"containers": []any{"DUMY0000001", "DUMY0000002"}},
		"not a list":                {"containers": "DUMY0000001"},
		"above the ceiling":         {"containers": tooMany},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := splitCtx(inputs)
			require.Error(t, SLPAContainerSplitBuilderFunc(ctx, nil))
			assert.NotContains(t, ctx.Record.Data, "split_items")
		})
	}
}

// Without a record there is nowhere to write the fan-out, and a nil dereference
// here would fail the activity with no account of what was wrong.
func TestSLPAContainerSplitBuilder_RefusesAMissingRecord(t *testing.T) {
	err := SLPAContainerSplitBuilderFunc(plugins.PluginContext{
		Inputs: map[string]any{"containers": containerOrder()},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task record is nil")
}
