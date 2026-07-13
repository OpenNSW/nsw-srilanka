package trade

import (
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/taskflow/plugins"
)

// hscodeSplitConfig is the plugin_properties schema for HSCODE_SPLIT_BUILDER.
// HsCodeFlows maps an HS code (as stored by the HS code selection form) to the
// workflow template IDs of the agency flows that must run for consignments
// carrying that code. SharedPayload entries are copied into every split item's
// payload — the engine surfaces them to each spawned flow at _iter.input.*,
// which is how sibling flows agree on things like signal names for
// sys:emit_signal / sys:wait_for_signal coordination.
type hscodeSplitConfig struct {
	HsCodeFlows   map[string][]string `json:"hs_code_flows"`
	SharedPayload map[string]any      `json:"shared_payload"`
}

// HscodeSplitBuilderFunc transforms the []string stored in hs_codes by the HS
// code selection form into the []map[string]any format that the
// go-temporal-workflow SPLIT_TASK node expects: [{template_id, branch_id, payload}].
//
// Each selected value is resolved through the hs_code_flows map in
// plugin_properties: an HS code expands to its configured workflow template
// IDs, while a value absent from the map is treated as a workflow template ID
// itself (legacy direct flow selection). Template IDs are deduplicated in
// first-seen order, and each split item's payload carries the HS codes that
// triggered that flow.
//
// The optional trader_company input (the company record injected into the
// trade workflow as traderCompany at consignment start) is copied into every
// split item's payload so spawned agency flows can prefill their forms from
// the company profile via _iter.input.trader_company.
//
// It is synchronous — it returns nil (not ErrSuspended) so the engine advances
// immediately without waiting for any user or external action. Register it via
// NewGenericExecutorPlugin.
func HscodeSplitBuilderFunc(ctx plugins.PluginContext, configRaw json.RawMessage) error {
	var cfg hscodeSplitConfig
	if len(configRaw) > 0 {
		if err := json.Unmarshal(configRaw, &cfg); err != nil {
			return fmt.Errorf("hscode_split_builder: invalid plugin_properties: %w", err)
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

	seen := make(map[string]bool)
	var templateIDs []string
	triggeringCodes := make(map[string][]string)
	for i, item := range items {
		value, ok := item.(string)
		if !ok {
			return fmt.Errorf("hscode_split_builder: item[%d] is not a string (got %T)", i, item)
		}
		flows, mapped := cfg.HsCodeFlows[value]
		if !mapped {
			flows = []string{value}
		}
		for _, templateID := range flows {
			if !seen[templateID] {
				seen[templateID] = true
				templateIDs = append(templateIDs, templateID)
			}
			if mapped {
				triggeringCodes[templateID] = append(triggeringCodes[templateID], value)
			}
		}
	}

	if len(templateIDs) == 0 {
		return fmt.Errorf("hscode_split_builder: selected HS codes resolved to no flows")
	}

	traderCompany := ctx.Inputs["trader_company"]

	splitItems := make([]map[string]any, 0, len(templateIDs))
	for _, templateID := range templateIDs {
		payload := make(map[string]any, len(cfg.SharedPayload)+2)
		for k, v := range cfg.SharedPayload {
			payload[k] = v
		}
		if traderCompany != nil {
			payload["trader_company"] = traderCompany
		}
		if codes := triggeringCodes[templateID]; len(codes) > 0 {
			payload["hs_codes"] = codes
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
