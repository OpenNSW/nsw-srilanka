package plugins

import (
	"encoding/json"
	"fmt"
	"strings"

	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
)

// TaskTypeSLPAGatePassSplitBuilder is the synchronous transform that turns the
// containers SLPA consolidated into the SPLIT_TASK items array.
const TaskTypeSLPAGatePassSplitBuilder = "SLPA_GATE_PASS_SPLIT_BUILDER"

// maxGatePassesPerOrder bounds the fan-out, as the CDN builder does: each item
// becomes a child workflow with a form for the trader to fill, so a runaway list
// from the CMS must not spawn an unbounded number of tasks. The ceiling is far
// above any real export service order.
const maxGatePassesPerOrder = 200

// SLPAGatePassSplitBuilderFunc expands the consolidated containers into the
// []map[string]any a SPLIT_TASK node consumes: one branch per container.
//
// A child workflow inherits nothing from its parent but the payload prepared
// here (see core/workflow dynamic_split.go), so each branch carries everything
// its gate-pass call needs — the container it is for, the service order slug the
// endpoint is scoped to, and the SLPA client key the CMS identifies the company
// by.
//
// It is synchronous: it returns nil (not ErrSuspended) so the engine advances
// immediately. Register it via trade.NewGenericExecutorPlugin.
func SLPAGatePassSplitBuilderFunc(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	if ctx.Record == nil {
		return fmt.Errorf("slpa_gate_pass_split_builder: task record is nil")
	}

	containers, err := consolidatedContainers(value(ctx, "containers"), value(ctx, "rows"))
	if err != nil {
		return err
	}

	slug, _ := value(ctx, "slug").(string)
	clientKey, _ := value(ctx, "client_key").(string)

	items := make([]map[string]any, 0, len(containers))
	for i, containerNo := range containers {
		items = append(items, map[string]any{
			// SAME_TEMPLATE mode takes the template from the SPLIT_TASK node and
			// suffixes this branch_id with the item index, so "gatepass" becomes
			// "gatepass-0", "gatepass-1", ….
			"branch_id": "gatepass",
			"payload": map[string]any{
				"container_no": containerNo,
				"slug":         slug,
				"client_key":   clientKey,
				// The trader fills one form per container, so each branch has to
				// say which container it is for; without it the passes are
				// indistinguishable on the dashboard.
				"container_label": fmt.Sprintf("Container %s (%d of %d)", containerNo, i+1, len(containers)),
			},
		})
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data["split_items"] = items
	return nil
}

// value reads one value the builder needs, from the step's mapped inputs first
// and the task record second.
//
// Both are the workflow's own doing — the record carries what the task node
// mapped in, the inputs what the subtask node mapped — and they are written by
// different mappings, so a value can legitimately be in one and not the other.
// Reading both means a step that has the value at all can use it, rather than
// failing on which of the two mappings named it.
func value(ctx flowplugins.PluginContext, key string) any {
	if v, ok := ctx.Inputs[key]; ok && v != nil {
		return v
	}
	if ctx.Record != nil && ctx.Record.Data != nil {
		return ctx.Record.Data[key]
	}
	return nil
}

// consolidatedContainers reads which containers are consolidated, from the two
// places that know: the rows the trader ticked on the consolidation form, and
// the containers SLPA had already paired before they got there.
//
// The trader's own selection is the source for the first — a container they
// declined is not consolidated, so no gate pass is issued for it.
//
// An empty list is an error rather than an empty fan-out: a SPLIT_TASK over zero
// items completes silently, which would leave the consignment with no gate pass
// and no explanation of why.
func consolidatedContainers(rawContainers, rawRows any) ([]string, error) {
	containers, err := containerList(rawContainers)
	if err != nil {
		return nil, err
	}
	selected, err := selectedContainers(rawRows)
	if err != nil {
		return nil, err
	}
	containers = append(containers, selected...)

	deduped := make([]string, 0, len(containers))
	seen := make(map[string]bool, len(containers))
	for _, containerNo := range containers {
		containerNo = strings.TrimSpace(containerNo)
		// A container listed twice would spawn two branches for one pass; SLPA
		// issues one per container.
		if containerNo == "" || seen[strings.ToUpper(containerNo)] {
			continue
		}
		seen[strings.ToUpper(containerNo)] = true
		deduped = append(deduped, containerNo)
	}

	if len(deduped) == 0 {
		return nil, fmt.Errorf("slpa_gate_pass_split_builder: no consolidated containers, so no gate pass can be issued")
	}
	if len(deduped) > maxGatePassesPerOrder {
		return nil, fmt.Errorf("slpa_gate_pass_split_builder: %d containers, above the supported maximum of %d",
			len(deduped), maxGatePassesPerOrder)
	}
	return deduped, nil
}

// containerList reads a plain list of container numbers, tolerating both the
// []any a task record holds after a round trip through JSON and the []string
// recorded in Go. A missing input is not an error here; the caller decides
// whether both sources being absent is one.
func containerList(raw any) ([]string, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []string:
		return v, nil
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("slpa_gate_pass_split_builder: containers is not a list (got %T)", raw)
	}
}

// selectedContainers reads the consolidation form's rows and returns the
// containers the trader ticked. An unticked row is a container they chose not to
// consolidate, so it gets no gate pass.
func selectedContainers(raw any) ([]string, error) {
	var items []any
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []any:
		items = v
	case []map[string]any:
		for _, item := range v {
			items = append(items, item)
		}
	default:
		return nil, fmt.Errorf("slpa_gate_pass_split_builder: rows is not a list (got %T)", raw)
	}

	var out []string
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ticked := false
		switch v := row["consolidate"].(type) {
		case bool:
			ticked = v
		case string:
			ticked = strings.EqualFold(strings.TrimSpace(v), "true")
		}
		if !ticked {
			continue
		}
		if no, ok := row["cap_container_no"].(string); ok {
			out = append(out, no)
		}
	}
	return out, nil
}
