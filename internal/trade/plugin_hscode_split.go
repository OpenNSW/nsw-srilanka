package trade

import (
	"encoding/json"
	"fmt"

	flowplugins "github.com/OpenNSW/nsw-task-flow/plugins"
)

// HscodeSplitBuilderFunc transforms a []string of workflow template IDs (stored in
// hs_codes by the HS code selection form) into the []map[string]any format that the
// go-temporal-workflow SPLIT_TASK node expects: [{template_id, branch_id, payload}].
//
// It is synchronous — it returns nil (not ErrSuspended) so the engine advances
// immediately without waiting for any user or external action. Register it via
// NewGenericExecutorPlugin.
func HscodeSplitBuilderFunc(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	raw, ok := ctx.Inputs["hs_codes"]
	if !ok {
		return fmt.Errorf("hscode_split_builder: hs_codes not found in inputs")
	}

	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("hscode_split_builder: hs_codes is not an array (got %T)", raw)
	}

	splitItems := make([]map[string]any, 0, len(items))
	for i, item := range items {
		templateID, ok := item.(string)
		if !ok {
			return fmt.Errorf("hscode_split_builder: item[%d] is not a string (got %T)", i, item)
		}
		splitItems = append(splitItems, map[string]any{
			"template_id": templateID,
			"branch_id":   templateID,
			"payload":     map[string]any{},
		})
	}

	ctx.Record.Data["split_items"] = splitItems
	return nil
}
