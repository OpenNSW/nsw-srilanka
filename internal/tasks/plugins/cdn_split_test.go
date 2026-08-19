package plugins

import (
	"fmt"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func splitCtx(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Inputs: inputs,
		Record: &store.TaskRecord{Data: map[string]any{}},
	}
}

func TestCDNSplitBuilder_OneBranchPerContainer(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"container_count": float64(3),
		"cusdec_ref":      "CBEX1/2026/E/1047",
	})
	require.NoError(t, CDNSplitBuilderFunc(ctx, nil))

	items, ok := ctx.Record.Data["split_items"].([]map[string]any)
	require.True(t, ok, "expected split_items, got %T", ctx.Record.Data["split_items"])
	require.Len(t, items, 3)

	for i, item := range items {
		// SAME_TEMPLATE mode suffixes branch_id with the index, so a constant
		// here still yields unique branches.
		assert.Equal(t, "cdn", item["branch_id"])

		payload := item["payload"].(map[string]any)
		assert.Equal(t, "CBEX1/2026/E/1047", payload["cusdec_ref"])
		assert.Equal(t, i+1, payload["container_sequence"])
		assert.Equal(t, 3, payload["container_count"])
		// The child workflow maps this straight onto the form's read-only
		// container field, so every branch must carry one.
		assert.Equal(t, fmt.Sprintf("Container %d of 3", i+1), payload["container_label"])
	}
}

// One container is the common case, not an edge case: the guard is count < 1,
// not count < 2. A single-container declaration must still raise its note.
func TestCDNSplitBuilder_SingleContainerRaisesOneNote(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"container_count": float64(1),
		"cusdec_ref":      "CBEX1/2026/E/1047",
	})
	require.NoError(t, CDNSplitBuilderFunc(ctx, nil))

	items := ctx.Record.Data["split_items"].([]map[string]any)
	require.Len(t, items, 1)
	payload := items[0]["payload"].(map[string]any)
	assert.Equal(t, 1, payload["container_sequence"])
	assert.Equal(t, "Container 1 of 1", payload["container_label"])
}

func TestCDNSplitBuilder_RejectsUnusableCounts(t *testing.T) {
	for name, inputs := range map[string]map[string]any{
		"missing":       {},
		"zero":          {"container_count": float64(0)},
		"negative":      {"container_count": float64(-1)},
		"not a number":  {"container_count": "many"},
		"above ceiling": {"container_count": float64(maxContainersPerDeclaration + 1)},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, CDNSplitBuilderFunc(splitCtx(inputs), nil))
		})
	}
}

// The declaration reference is optional at split time: the dispatch step is
// where its absence becomes a trader-facing error.
func TestCDNSplitBuilder_AllowsMissingCusdecRef(t *testing.T) {
	ctx := splitCtx(map[string]any{"container_count": float64(1)})
	require.NoError(t, CDNSplitBuilderFunc(ctx, nil))

	items := ctx.Record.Data["split_items"].([]map[string]any)
	require.Len(t, items, 1)
	assert.Equal(t, "", items[0]["payload"].(map[string]any)["cusdec_ref"])
}
