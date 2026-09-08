package plugins

import (
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capContainerRows is the pre-advised side as the lookup records it, in the
// []any shape a task record holds once it has been through storage.
func capContainerRows() []any {
	return []any{
		map[string]any{"sqid": "cap-A", "container_no": "MSCU8492019", "so_container_sqid": nil},
		map[string]any{"sqid": "cap-B", "container_no": "TCLU1234567", "so_container_sqid": nil},
	}
}

// The trader picks once, from a list whose values are sqids. A gate pass is
// requested by container number, and the form they read next has to name the
// container it is for, so both names are recorded here — a workflow mapping can
// copy a value but not look one up.
func TestSLPAConsolidationResolve_RecordsBothNamesOfTheChosenContainer(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"cap_container_no": "cap-B",
		"cap_containers":   capContainerRows(),
	})
	require.NoError(t, SLPAConsolidationResolveFunc(ctx, nil))

	assert.Equal(t, "cap-B", ctx.Record.Data["cap_sqid"])
	assert.Equal(t, "TCLU1234567", ctx.Record.Data["container_no"])
}

// The branch redoes a consolidation by reading a flag the delete sets, and the
// gate pass does not set it when it issues a pass. Since a workflow output
// mapping can only copy a value it finds, an absent flag left the previous one
// standing: once a trader had deleted a pairing, every pass they went on to
// issue sent the branch back to consolidate, with no way to finish the
// container. Resolving says the answer plainly instead.
func TestSLPAConsolidationResolve_ClearsTheDeletedFlagItRedoesOn(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"cap_container_no": "cap-A",
		"cap_containers":   capContainerRows(),
		// What the branch carries after a delete-and-redo, which is the state
		// that used to strand it.
		"deleted": true,
	})
	require.NoError(t, SLPAConsolidationResolveFunc(ctx, nil))

	assert.Equal(t, false, ctx.Record.Data["deleted"],
		"a resolved pairing is not a deleted one, so the branch can reach its gate pass")
}

// Nothing is recorded for a resolve that failed, the flag included: saying "not
// deleted" about a pairing that was never resolved would route the branch on to
// a gate pass for a container it could not name.
func TestSLPAConsolidationResolve_LeavesTheFlagAloneWhenItCannotResolve(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"cap_container_no": "cap-gone",
		"cap_containers":   capContainerRows(),
		"deleted":          true,
	})
	require.Error(t, SLPAConsolidationResolveFunc(ctx, nil))
	assert.NotContains(t, ctx.Record.Data, "deleted")
}

// A caller holding only the number should not have to look the sqid up first,
// so the choice is matched on either name, ignoring case as SLPA's own values
// have been seen to differ in it.
func TestSLPAConsolidationResolve_AcceptsAChoiceGivenAsAContainerNumber(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"cap_container_no": "tclu1234567",
		"cap_containers":   capContainerRows(),
	})
	require.NoError(t, SLPAConsolidationResolveFunc(ctx, nil))

	assert.Equal(t, "cap-B", ctx.Record.Data["cap_sqid"])
	assert.Equal(t, "TCLU1234567", ctx.Record.Data["container_no"])
}

// The rows reach this step either straight from the lookup, still typed, or off
// the record as []any. Reading only the latter found no rows in the former and
// reported the container as gone — immediately after a save the CMS had
// accepted, with the pairing already written on SLPA's side.
func TestSLPAConsolidationResolve_ReadsBothShapesTheRowsArriveIn(t *testing.T) {
	ctx := splitCtx(map[string]any{
		"cap_container_no": "cap-B",
		"cap_containers": []map[string]any{
			{"sqid": "cap-A", "container_no": "MSCU8492019"},
			{"sqid": "cap-B", "container_no": "TCLU1234567"},
		},
	})
	require.NoError(t, SLPAConsolidationResolveFunc(ctx, nil))

	assert.Equal(t, "cap-B", ctx.Record.Data["cap_sqid"])
	assert.Equal(t, "TCLU1234567", ctx.Record.Data["container_no"])
}

// The task node and the subtask node are two different mappings, so a value can
// be on the record without being among the step's inputs.
func TestSLPAConsolidationResolve_ReadsTheRecordWhenInputsAreBare(t *testing.T) {
	ctx := splitCtx(map[string]any{})
	ctx.Record.Data["cap_container_no"] = "cap-A"
	ctx.Record.Data["cap_containers"] = capContainerRows()
	require.NoError(t, SLPAConsolidationResolveFunc(ctx, nil))

	assert.Equal(t, "cap-A", ctx.Record.Data["cap_sqid"])
	assert.Equal(t, "MSCU8492019", ctx.Record.Data["container_no"])
}

// Failing here says so while the trader is still on the container, rather than
// letting a gate pass go out naming nothing.
func TestSLPAConsolidationResolve_RefusesWhatItCannotResolve(t *testing.T) {
	for name, tc := range map[string]struct {
		inputs map[string]any
		reason string
	}{
		"nothing chosen": {
			inputs: map[string]any{"cap_containers": capContainerRows()},
			reason: "no container was chosen",
		},
		"a blank choice": {
			inputs: map[string]any{"cap_container_no": "   ", "cap_containers": capContainerRows()},
			reason: "no container was chosen",
		},
		"a choice SLPA no longer offers": {
			inputs: map[string]any{"cap_container_no": "cap-gone", "cap_containers": capContainerRows()},
			reason: "no longer holds the container",
		},
		"no rows to resolve against": {
			inputs: map[string]any{"cap_container_no": "cap-A", "cap_containers": []any{}},
			reason: "no longer holds the container",
		},
		"a row missing its number": {
			inputs: map[string]any{
				"cap_container_no": "cap-A",
				"cap_containers":   []any{map[string]any{"sqid": "cap-A"}},
			},
			reason: "no longer holds the container",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := splitCtx(tc.inputs)
			err := SLPAConsolidationResolveFunc(ctx, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.reason)
			assert.NotContains(t, ctx.Record.Data, "cap_sqid")
			assert.NotContains(t, ctx.Record.Data, "container_no")
		})
	}
}

func TestSLPAConsolidationResolve_RefusesAMissingRecord(t *testing.T) {
	err := SLPAConsolidationResolveFunc(plugins.PluginContext{
		Inputs: map[string]any{"cap_container_no": "cap-A", "cap_containers": capContainerRows()},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task record is nil")
}
