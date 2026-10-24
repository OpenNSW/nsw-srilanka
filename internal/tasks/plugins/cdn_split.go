package plugins

import (
	"encoding/json"
	"fmt"

	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
)

// TaskTypeCDNSplitBuilder is the synchronous transform that turns a
// declaration's container count into the SPLIT_TASK items array.
const TaskTypeCDNSplitBuilder = "CDN_SPLIT_BUILDER"

// maxContainersPerDeclaration bounds the fan-out. Each item becomes a child
// workflow with a trader form to fill, so a mistyped container count would
// otherwise spawn an unbounded number of tasks against the trader's dashboard.
// The ceiling is far above any real export consignment.
const maxContainersPerDeclaration = 200

// CDNSplitBuilderFunc expands a declaration's container count into the
// []map[string]any that a SPLIT_TASK node consumes: one branch per container,
// each carrying the declaration reference and its own container sequence.
//
// One CDN covers exactly one container — Annex B's container block is a single
// object — so the number of dispatch notes a consignment needs is the
// numberOfContainers the trader declared on the CusDec. Deriving the fan-out
// here rather than asking for it again means the two can never disagree.
//
// It is synchronous: it returns nil (not ErrSuspended) so the engine advances
// immediately. Register it via trade.NewGenericExecutorPlugin.
func CDNSplitBuilderFunc(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	count, err := containerCount(ctx.Inputs)
	if err != nil {
		return err
	}

	// The registered reference is what each CDN has to quote in cusDecRefs. It
	// is optional here so a consignment can still be split before the reference
	// lands; the dispatch step reports its absence to the trader.
	cusdecRef, _ := ctx.Inputs["cusdec_ref"].(string)

	items := make([]map[string]any, 0, count)
	for i := 1; i <= count; i++ {
		items = append(items, map[string]any{
			// SAME_TEMPLATE mode takes the template from the SPLIT_TASK node and
			// suffixes this branch_id with the item index, so "cdn" becomes
			// "cdn-0", "cdn-1", ….
			"branch_id": "cdn",
			"payload": map[string]any{
				"cusdec_ref":         cusdecRef,
				"container_sequence": i,
				"container_count":    count,
				// The trader fills one form per container, so each has to say
				// which container it is for; without it the notes are
				// indistinguishable on the dashboard.
				"container_label": fmt.Sprintf("Container %d of %d", i, count),
			},
		})
	}

	if ctx.Record == nil {
		return fmt.Errorf("cdn_split_builder: task record is nil")
	}
	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data["split_items"] = items
	return nil
}

// containerCount reads the declared container count from the task inputs. The
// form declares it as a number and the value reaches this plugin as decoded
// JSON, so it is a float64; int is accepted for a caller that builds the inputs
// in Go rather than through a workflow.
func containerCount(inputs map[string]any) (int, error) {
	raw, ok := inputs["container_count"]
	if !ok {
		return 0, fmt.Errorf("cdn_split_builder: container_count not found in inputs")
	}

	var count int
	switch v := raw.(type) {
	case float64:
		count = int(v)
	case int:
		count = v
	default:
		return 0, fmt.Errorf("cdn_split_builder: container_count is not a number (got %T)", raw)
	}

	// A containerised declaration that reached this step with no containers has
	// nothing to dispatch, and a SPLIT_TASK over zero items would silently
	// complete — leaving the consignment with no CDN and no explanation.
	if count < 1 {
		return 0, fmt.Errorf("cdn_split_builder: declaration reports %d containers, so no dispatch note can be raised", count)
	}
	if count > maxContainersPerDeclaration {
		return 0, fmt.Errorf("cdn_split_builder: declaration reports %d containers, above the supported maximum of %d",
			count, maxContainersPerDeclaration)
	}
	return count, nil
}
