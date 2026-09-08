package plugins

import (
	"encoding/json"
	"fmt"
	"strings"

	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/consolidation"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
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
	for _, row := range fields.Rows(value(ctx, consolidation.CapContainersKey)) {
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

	// This container is consolidated, so nothing about it is deleted. Said
	// plainly because the branch decides whether to redo a consolidation by
	// reading a flag the delete sets, and a workflow output mapping can only
	// copy a value it finds: the gate pass step does not set that flag when it
	// issues a pass, so an optional mapping leaves whatever was there before.
	// Once a trader had deleted a pairing once, the flag stayed true and the
	// branch was sent back to consolidate after every pass it issued, with no
	// way to finish the container. Resolving a pairing is the moment the answer
	// is knowably false, so it is recorded here rather than left to the absence
	// of a value elsewhere.
	ctx.Record.Data["deleted"] = false
	return nil
}

// asString reads a value the workflow recorded as text.
func asString(v any) string {
	s, _ := v.(string)
	return s
}
