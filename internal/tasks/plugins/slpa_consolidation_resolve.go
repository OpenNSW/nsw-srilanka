package plugins

import (
	"encoding/json"
	"fmt"
	"strings"

	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/consolidation"
)

// TaskTypeSLPAConsolidationResolve is the synchronous transform that turns the
// container the trader picked into the two things the steps after it need.
const TaskTypeSLPAConsolidationResolve = "SLPA_CONSOLIDATION_RESOLVE"

// SLPAConsolidationResolveFunc records the chosen container by both of its
// names: the sqid SLPA addresses it by, and the number written on the box.
//
// The trader picks once, from a list whose values are sqids — one answer that
// keys both the save and the delete. But a gate pass is requested by container
// number, and the trader reading the gate-pass form needs to see which container
// it is for. Neither can be derived from the other by a workflow mapping, which
// can copy a value but not look one up, so the pairing is resolved here against
// what the lookup recorded.
//
// It is synchronous: it returns nil (not ErrSuspended) so the engine advances
// immediately. Register it via trade.NewGenericExecutorPlugin.
func SLPAConsolidationResolveFunc(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	if ctx.Record == nil {
		return fmt.Errorf("slpa_consolidation_resolve: task record is nil")
	}

	chosen := strings.TrimSpace(asString(value(ctx, consolidation.ChosenCapKey)))
	if chosen == "" {
		return fmt.Errorf("slpa_consolidation_resolve: no container was chosen, so there is nothing to resolve")
	}

	sqid, number := chosen, ""
	for _, raw := range asAnyList(value(ctx, consolidation.CapContainersKey)) {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rowSqid := strings.TrimSpace(asString(row["sqid"]))
		rowNo := strings.TrimSpace(asString(row["container_no"]))
		if rowSqid == chosen || strings.EqualFold(rowNo, chosen) {
			sqid, number = rowSqid, rowNo
			break
		}
	}
	if number == "" {
		// The choice is no longer among what SLPA offered. Failing here says so
		// while the trader is still on the container, rather than letting a gate
		// pass go out naming nothing.
		return fmt.Errorf("slpa_consolidation_resolve: SLPA no longer holds the container that was consolidated (%s)", chosen)
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data["cap_sqid"] = sqid
	ctx.Record.Data["container_no"] = number
	return nil
}

// asString reads a value the workflow recorded as text.
func asString(v any) string {
	s, _ := v.(string)
	return s
}

// asAnyList reads a value the workflow recorded as a list, tolerating the []any
// a task record holds after a round trip through JSON.
func asAnyList(v any) []any {
	items, _ := v.([]any)
	return items
}
