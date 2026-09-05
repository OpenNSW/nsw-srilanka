package plugins

import (
	"encoding/json"
	"fmt"
	"strings"

	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// TaskTypeSLPAContainerSplitBuilder is the synchronous transform that turns the
// containers the trader put on the service order into the SPLIT_TASK items
// array — one branch per container.
const TaskTypeSLPAContainerSplitBuilder = "SLPA_CONTAINER_SPLIT_BUILDER"

// maxContainersPerOrder bounds the fan-out, as the CDN builder does: each item
// becomes a child workflow with forms for the trader to fill, so a runaway list
// must not spawn an unbounded number of tasks.
const maxContainersPerOrder = 200

// SLPAContainerSplitBuilderFunc expands the service order's containers into the
// []map[string]any a SPLIT_TASK node consumes.
//
// The count comes from the order the trader raised, not from what SLPA is
// holding: the order says how many containers this consignment is for, while
// the terminal pre-advises the real ones as they arrive. A branch therefore
// exists from the start for a container whose real number does not exist yet,
// which is what lets the trader come back to that one task once they have
// pre-advised it in Navis.
//
// Each branch is one placeholder — the dummy container number the order was
// priced against. Owning it outright is what keeps two branches from claiming
// the same one, and it is the fixed side of the pairing the trader completes by
// choosing a real container.
//
// A child workflow inherits nothing from its parent but the payload prepared
// here (see core/workflow dynamic_split.go), so each branch carries everything
// its calls need: the placeholder, the declaration the lookup is keyed on, the
// order slug the gate-pass endpoint is scoped to, and the client key the CMS
// identifies the company by.
//
// It is synchronous: it returns nil (not ErrSuspended) so the engine advances
// immediately. Register it via trade.NewGenericExecutorPlugin.
func SLPAContainerSplitBuilderFunc(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	if ctx.Record == nil {
		return fmt.Errorf("slpa_container_split_builder: task record is nil")
	}

	rows, err := containerRows(value(ctx, "containers"))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		// A SPLIT_TASK over zero items completes silently, which would leave the
		// consignment with nothing consolidated, no gate pass, and no account of
		// why. An order with no containers cannot have been raised at all.
		return fmt.Errorf("slpa_container_split_builder: the service order names no containers to consolidate")
	}
	if len(rows) > maxContainersPerOrder {
		return fmt.Errorf("slpa_container_split_builder: %d containers, above the supported maximum of %d",
			len(rows), maxContainersPerOrder)
	}

	cusdec, _ := value(ctx, "cusdec_serial").(string)
	slug, _ := value(ctx, "slug").(string)
	clientKey, _ := value(ctx, "client_key").(string)

	items := make([]map[string]any, 0, len(rows))
	for i, row := range rows {
		placeholder := strings.TrimSpace(fields.String(row, "containerNo"))
		items = append(items, map[string]any{
			// SAME_TEMPLATE mode takes the template from the SPLIT_TASK node and
			// suffixes this branch_id with the item index, so "container" becomes
			// "container-0", "container-1", ….
			"branch_id": "container",
			"payload": map[string]any{
				"so_container_no": placeholder,
				// Seeded so every gateway in the branch has a value to read from
				// the first pass: a condition on a variable the child has never
				// been given is an expression that cannot compile, which parks
				// the branch instead of routing it.
				"cap_container_no": "",
				"deleted":          false,
				"container_size":   fields.String(row, "containerSize"),
				"cusdec_serial":    cusdec,
				"slug":             slug,
				"client_key":       clientKey,
				// The trader works one container at a time, so each branch has to
				// say which it is for; without it they are indistinguishable on
				// the dashboard.
				"container_label": containerBranchLabel(placeholder, i, len(rows)),
			},
		})
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data["split_items"] = items
	return nil
}

// containerBranchLabel names a branch for the trader's task list. A placeholder
// with no number falls back to its position, so a row the order accepted but
// this cannot read is still findable rather than nameless.
func containerBranchLabel(placeholder string, index, total int) string {
	if placeholder == "" {
		return fmt.Sprintf("Container %d of %d", index+1, total)
	}
	return fmt.Sprintf("Container %s (%d of %d)", placeholder, index+1, total)
}

// containerRows reads the service order's container rows, tolerating both the
// []any a task record holds after a round trip through JSON and the typed slice
// recorded in Go.
func containerRows(raw any) ([]map[string]any, error) {
	var items []any
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []any:
		items = v
	case []map[string]any:
		return v, nil
	default:
		return nil, fmt.Errorf("slpa_container_split_builder: containers is not a list (got %T)", raw)
	}

	out := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		if row, ok := raw.(map[string]any); ok {
			out = append(out, row)
		}
	}
	return out, nil
}
