package plugins

import (
	"encoding/json"
	"fmt"

	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
)

// TaskTypeCDNResultsCollector is the synchronous transform that folds a
// SPLIT_TASK's per-container results back into single workflow variables.
const TaskTypeCDNResultsCollector = "CDN_RESULTS_COLLECTOR"

// CDNResultsCollectorFunc reduces the aggregated output of the per-container CDN
// fan-out into the variables the steps after it read.
//
// A SPLIT_TASK writes its results as an array — one entry per branch, holding
// that child workflow's whole variable set. Workflow input mappings resolve
// dot-paths through maps only and cannot index an array, so nothing downstream
// can reach into that structure on its own. This flattens it:
//
//   - cdn_numbers: every branch's dispatch note, in container order.
//   - containers: one boat-note form row per registered note, in that same
//     order, so the boat-note USER_INPUT can bind the array as-is. JSON Forms
//     cannot size an array from a sibling field, and input_mapping cannot
//     loop, so the rows have to exist before that form opens. Unregistered
//     and failed branches are omitted: the form locks the CDN string and
//     requires it, so a blank row would either force the trader to invent a
//     number Customs never registered, or block submit once the field is
//     read-only.
//   - cdn_userform / cdn_number: the first accepted note, so the
//     acknowledgement step can keep the single-note input it was built for.
func CDNResultsCollectorFunc(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	branches, err := branchResults(ctx.Inputs)
	if err != nil {
		return err
	}

	var (
		numbers []any
		firstOK map[string]any
	)
	containers := make([]map[string]any, 0, len(branches))

	for _, branch := range branches {
		userform, _ := branch["userform"].(map[string]any)
		cig, _ := branch["cig_cdn"].(map[string]any)

		accepted, _ := cig["accepted"].(bool)

		// Only the reference Customs registered counts. Downstream steps quote it
		// back to Customs, so the trader's own note number — which the form also
		// collects, for the printed note — would be a reference they cannot
		// resolve. A branch with no registered number has no dispatch note at
		// Customs, and contributes nothing to cdn_numbers or containers — the
		// boat-note form cannot add, remove, or edit the CDN string, so a blank
		// row would be unsubmittable.
		if num := userform["registeredCdnNumber"]; num != nil && num != "" {
			numbers = append(numbers, num)
			containers = append(containers, map[string]any{"cdn_number": num})
		}
		if accepted && firstOK == nil {
			firstOK = userform
		}
	}

	// COLLECT_ALL lets a branch finish rejected, so there may be no accepted
	// note at all. Falling back to the first branch keeps the downstream steps
	// populated with something the trader recognizes rather than nothing.
	if firstOK == nil && len(branches) > 0 {
		firstOK, _ = branches[0]["userform"].(map[string]any)
	}

	if ctx.Record == nil {
		return fmt.Errorf("cdn_results_collector: task record is nil")
	}
	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data["cdn_numbers"] = numbers
	ctx.Record.Data["containers"] = containers
	ctx.Record.Data["cdn_userform"] = firstOK

	// cdn_number is always written, even with nothing to write. Steps that
	// still quote a single note (acknowledgement) map it as required, and a
	// required mapping that resolves to nothing parks the consignment — an
	// empty string carries the same "no note" meaning without stalling the flow.
	ctx.Record.Data["cdn_number"] = ""
	if len(numbers) > 0 {
		ctx.Record.Data["cdn_number"] = numbers[0]
	}
	return nil
}

// branchResults reads the SPLIT_TASK results array from the task inputs. Each
// entry is one branch's workflow variables; a branch that halted carries an
// "error" key instead, and is counted rather than skipped silently.
func branchResults(inputs map[string]any) ([]map[string]any, error) {
	raw, ok := inputs["cdn_results"]
	if !ok {
		return nil, fmt.Errorf("cdn_results_collector: cdn_results not found in inputs")
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("cdn_results_collector: cdn_results is not an array (got %T)", raw)
	}

	branches := make([]map[string]any, 0, len(list))
	for i, entry := range list {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cdn_results_collector: cdn_results[%d] is not an object (got %T)", i, entry)
		}
		branches = append(branches, m)
	}
	return branches, nil
}
