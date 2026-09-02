package trade

import (
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/taskflow/plugins"
)

type hscodeSplitConfig struct {
	Mappings map[string][]string `json:"mappings"`
}

// HscodeSplitBuilderFunc transforms a list of HS codes (stored in hs_codes by the
// HS code selection form) into the []map[string]any format that the go-temporal-workflow
// SPLIT_TASK node expects: [{template_id, branch_id, payload}].
//
// The mapping of each HS code to its corresponding set of workflow template IDs is strictly
// driven by plugin_properties.mappings in the task template config artifact (transform.json).
// If an unmapped HS code is requested, it fails with an error.
//
// Each item carries the submitting company in its payload when one is mapped in.
// A branch child is started with only its iteration context — none of the parent
// workflow's variables — so anything an agency flow needs about the consignment
// has to travel this way. Agencies read what they need from it (SLPA, for one,
// takes the CMS client key SLPA issued to the company); this builder does not
// interpret it, so adding a field is an artifact change rather than a code one.
//
// It is synchronous — it returns nil (not ErrSuspended) so the engine advances
// immediately without waiting for any user or external action. Register it via
// NewGenericExecutorPlugin.
func HscodeSplitBuilderFunc(ctx plugins.PluginContext, config json.RawMessage) error {
	var cfg hscodeSplitConfig
	if len(config) > 0 && string(config) != "null" {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("hscode_split_builder: invalid config: %w", err)
		}
	}

	raw, ok := ctx.Inputs["hs_codes"]
	if !ok {
		return fmt.Errorf("hscode_split_builder: hs_codes not found in inputs")
	}

	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("hscode_split_builder: hs_codes is not an array (got %T)", raw)
	}
	if len(items) == 0 {
		return fmt.Errorf("hscode_split_builder: hs_codes cannot be empty")
	}

	// Optional: a consignment whose artifacts need no company data maps none in.
	company, _ := ctx.Inputs["company"].(map[string]any)

	templates, err := resolveTemplates(items, cfg.Mappings)
	if err != nil {
		return err
	}

	splitItems := make([]map[string]any, 0, len(templates))
	for _, templateID := range templates {
		payload := map[string]any{}
		if company != nil {
			payload["company"] = company
		}
		splitItems = append(splitItems, map[string]any{
			"template_id": templateID,
			"branch_id":   templateID,
			"payload":     payload,
		})
	}

	if ctx.Record == nil {
		return fmt.Errorf("hscode_split_builder: task record is nil")
	}
	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data["split_items"] = splitItems
	return nil
}

func resolveTemplates(items []any, mappings map[string][]string) ([]string, error) {
	templates := make([]string, 0)
	seen := make(map[string]bool)

	for i, item := range items {
		hsCode, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("hscode_split_builder: item[%d] is not a string (got %T)", i, item)
		}

		workflows, exists := mappings[hsCode]
		if !exists || len(workflows) == 0 {
			return nil, fmt.Errorf("hscode_split_builder: unmapped HS code %q", hsCode)
		}

		for _, wf := range workflows {
			if !seen[wf] {
				seen[wf] = true
				templates = append(templates, wf)
			}
		}
	}
	return templates, nil
}
