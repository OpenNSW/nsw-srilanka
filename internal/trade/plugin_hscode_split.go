package trade

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/OpenNSW/core/taskflow/plugins"
)

type hscodeSplitConfig struct {
	Mappings map[string][]string `json:"mappings"`
	// Replaces names, for a workflow, the workflows it takes the place of: when a
	// consignment starts it, those are not started, whichever HS codes asked for
	// them. A flow that does another's work for some goods (the tea flow submits the
	// CusDec that customs-workflow would) replaces it for the whole consignment, so a
	// consignment mixing those goods with others still gets one of them.
	Replaces map[string][]string `json:"replaces"`
}

// HscodeSplitBuilderFunc transforms a list of HS codes (stored in hs_codes by the
// HS code selection form) into the []map[string]any format that the go-temporal-workflow
// SPLIT_TASK node expects: [{template_id, branch_id, payload}].
//
// The mapping of each HS code to its corresponding set of workflow template IDs is strictly
// driven by plugin_properties.mappings in the task template config artifact (transform.json).
// If an unmapped HS code is requested, it fails with an error. plugin_properties.replaces
// then drops the workflows that a selected workflow takes the place of.
//
// Each item's payload carries every input other than hs_codes, under its own name: the
// submitting company, the CHA chosen for the consignment, and whatever else the
// trade workflow maps in, such as the consignment's CusDec registration number.
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
	if len(config) == 0 {
		return fmt.Errorf("hscode_split_builder: config is required")
	}

	var cfg hscodeSplitConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("hscode_split_builder: invalid config: %w", err)
	}
	if len(cfg.Mappings) == 0 {
		return fmt.Errorf("hscode_split_builder: no mappings configured")
	}
	if err := validateReplaces(cfg.Replaces); err != nil {
		return err
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

	shared := sharedPayload(ctx.Inputs)

	templates, err := resolveTemplates(items, cfg.Mappings)
	if err != nil {
		return err
	}
	templates = applyReplaces(templates, cfg.Replaces)

	splitItems := make([]map[string]any, 0, len(templates))
	for _, templateID := range templates {
		payload := maps.Clone(shared)
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

// sharedPayload is what every agency flow gets: each input the node maps in other
// than hs_codes. An input with no value, such as a consignment without a CHA, is
// left out.
func sharedPayload(inputs map[string]any) map[string]any {
	shared := make(map[string]any, len(inputs))
	for name, value := range inputs {
		if name == "hs_codes" || value == nil {
			continue
		}
		shared[name] = value
	}
	return shared
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
			if wf == "" {
				return nil, fmt.Errorf("hscode_split_builder: HS code %q has an empty workflow template ID", hsCode)
			}
			if !seen[wf] {
				seen[wf] = true
				templates = append(templates, wf)
			}
		}
	}
	return templates, nil
}

// validateReplaces rejects a replaces rule with no single reading: a workflow
// replacing itself, or one that replaces others while another replaces it, which
// leaves open whether a workflow that is dropped still drops the ones it replaces.
func validateReplaces(replaces map[string][]string) error {
	replaced := make(map[string]bool)
	for wf, others := range replaces {
		for _, other := range others {
			if other == "" {
				return fmt.Errorf("hscode_split_builder: %q replaces an empty workflow template ID", wf)
			}
			if other == wf {
				return fmt.Errorf("hscode_split_builder: %q replaces itself", wf)
			}
			replaced[other] = true
		}
	}
	for wf := range replaces {
		if replaced[wf] {
			return fmt.Errorf("hscode_split_builder: %q both replaces workflows and is replaced", wf)
		}
	}
	return nil
}

// applyReplaces drops every workflow that a workflow in templates replaces,
// keeping the order of the rest.
func applyReplaces(templates []string, replaces map[string][]string) []string {
	dropped := make(map[string]bool)
	for _, wf := range templates {
		for _, other := range replaces[wf] {
			dropped[other] = true
		}
	}
	kept := make([]string, 0, len(templates))
	for _, wf := range templates {
		if !dropped[wf] {
			kept = append(kept, wf)
		}
	}
	return kept
}
