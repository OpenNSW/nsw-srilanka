package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// branch builds one SPLIT_TASK branch result: a child workflow's variables.
func branch(cdnNumber, edgeID string, accepted bool) map[string]any {
	return map[string]any{
		"userform": map[string]any{
			// What Customs registered on the §7.2 result.
			"registeredCdnNumber": cdnNumber,
			// The trader's own note number, deliberately different: it must
			// never be mistaken for the registered reference.
			"noteDetails": map[string]any{"cdnNumber": "TRADER-" + cdnNumber},
			"container":   map[string]any{"number": "MSCU" + cdnNumber},
		},
		"cig_cdn": map[string]any{"accepted": accepted, "edgeId": edgeID},
	}
}

// Only the registered reference is collected. A note whose registration never
// came back has no number Customs can resolve, so it contributes none.
func TestCDNResultsCollector_IgnoresUnregisteredNotes(t *testing.T) {
	registered := branch("CBEX1/2026/C/28237", "edge-1", true)

	unregistered := branch("unused", "edge-2", true)
	delete(unregistered["userform"].(map[string]any), "registeredCdnNumber")

	ctx := splitCtx(map[string]any{"cdn_results": []any{registered, unregistered}})
	require.NoError(t, CDNResultsCollectorFunc(ctx, nil))

	d := ctx.Record.Data
	assert.Equal(t, []any{"CBEX1/2026/C/28237"}, d["cdn_numbers"],
		"the trader's own note number must never stand in for a registered one")
	assert.Equal(t, "CBEX1/2026/C/28237", d["cdn_number"])
}

func TestCDNResultsCollector_FlattensEveryBranch(t *testing.T) {
	ctx := splitCtx(map[string]any{"cdn_results": []any{
		branch("CDN-1", "edge-1", true),
		branch("CDN-2", "edge-2", true),
		branch("CDN-3", "edge-3", true),
	}})
	require.NoError(t, CDNResultsCollectorFunc(ctx, nil))

	d := ctx.Record.Data
	assert.Equal(t, []any{"CDN-1", "CDN-2", "CDN-3"}, d["cdn_numbers"])

	// Steps still shaped around one note read the first accepted one.
	assert.Equal(t, "CDN-1", d["cdn_number"])
	require.NotNil(t, d["cdn_userform"])
}

// COLLECT_ALL lets a container's note fail without abandoning the others, so
// the first *accepted* note is the one downstream steps act on.
func TestCDNResultsCollector_SkipsRejectedForFirstNote(t *testing.T) {
	ctx := splitCtx(map[string]any{"cdn_results": []any{
		branch("CDN-1", "edge-1", false),
		branch("CDN-2", "edge-2", true),
	}})
	require.NoError(t, CDNResultsCollectorFunc(ctx, nil))

	d := ctx.Record.Data
	userform := d["cdn_userform"].(map[string]any)
	assert.Equal(t, "CDN-2", userform["registeredCdnNumber"])
}

func TestCDNResultsCollector_AllRejectedStillPopulates(t *testing.T) {
	ctx := splitCtx(map[string]any{"cdn_results": []any{branch("CDN-1", "edge-1", false)}})
	require.NoError(t, CDNResultsCollectorFunc(ctx, nil))

	d := ctx.Record.Data
	require.NotNil(t, d["cdn_userform"], "downstream steps must not be handed a nil form")
	// The boat-note step maps cdn_number as required, so it must always exist.
	assert.Equal(t, "CDN-1", d["cdn_number"])
}

// With no dispatch note number anywhere, cdn_number must still be written: a
// required mapping that resolves to nothing parks the consignment.
func TestCDNResultsCollector_AlwaysEmitsCDNNumber(t *testing.T) {
	ctx := splitCtx(map[string]any{"cdn_results": []any{
		map[string]any{"error": "child workflow halted", "branch_id": "cdn-0"},
	}})
	require.NoError(t, CDNResultsCollectorFunc(ctx, nil))

	d := ctx.Record.Data
	require.Contains(t, d, "cdn_number")
	assert.Equal(t, "", d["cdn_number"])
}

// A branch that halted carries an error instead of variables; it must be
// counted as rejected rather than crashing the collector.
func TestCDNResultsCollector_HandlesFailedBranch(t *testing.T) {
	ctx := splitCtx(map[string]any{"cdn_results": []any{
		map[string]any{"error": "child workflow halted", "branch_id": "cdn-0"},
		branch("CDN-2", "edge-2", true),
	}})
	require.NoError(t, CDNResultsCollectorFunc(ctx, nil))

	d := ctx.Record.Data
	assert.Equal(t, []any{"CDN-2"}, d["cdn_numbers"])
	assert.Equal(t, "CDN-2", d["cdn_number"])
}

func TestCDNResultsCollector_RejectsUnusableInput(t *testing.T) {
	assert.Error(t, CDNResultsCollectorFunc(splitCtx(map[string]any{}), nil))
	assert.Error(t, CDNResultsCollectorFunc(splitCtx(map[string]any{"cdn_results": "nope"}), nil))
	assert.Error(t, CDNResultsCollectorFunc(splitCtx(map[string]any{"cdn_results": []any{"nope"}}), nil))
}
