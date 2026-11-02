package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// TaskTypeSLPACaseContainers lists the containers a consignment registered with
// SLPA, for a form that orders something against one of them.
const TaskTypeSLPACaseContainers = "SLPA_CASE_CONTAINERS"

// CaseTasks lists the tasks of a case. The task store satisfies it.
type CaseTasks interface {
	GetAllTasks(ctx context.Context, caseID string) []store.TaskRecord
}

// SLPACaseContainersPlugin reads the consignment's containers from the case's
// own tasks.
//
// A sundry order runs as a flow of its own on the consignment, and a flow starts
// with nothing of another flow's variables. What the trader registered is still
// on the case, though: every task of every flow is filed under the consignment,
// so the containers are read from the task records rather than asked for again.
type SLPACaseContainersPlugin struct {
	tasks CaseTasks
}

// NewSLPACaseContainersPlugin returns the plugin, reading the case's tasks from
// tasks.
func NewSLPACaseContainersPlugin(tasks CaseTasks) *SLPACaseContainersPlugin {
	return &SLPACaseContainersPlugin{tasks: tasks}
}

// slpaCaseContainersConfig names the task templates the containers are read
// from, so a renamed template is a change to the artifact, not to this.
type slpaCaseContainersConfig struct {
	// RegistrationTemplateID is the ECDN task, whose form lists each
	// container by the dummy number SLPA keys the order by.
	RegistrationTemplateID string `json:"registration_template_id"`
	// PairingTemplateIDs are the tasks that record which real container a dummy
	// number was consolidated to, and whether that pairing was since deleted.
	PairingTemplateIDs []string `json:"pairing_template_ids"`
}

// Execute records the case's containers under the output namespace as
// containers: one row per dummy number, in the order they were registered, with
// the real container number it is consolidated to, or "" while it is not.
//
// A case with nothing registered yet records an empty list rather than failing:
// the form still opens, and the trader can type the number as before.
func (p *SLPACaseContainersPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg slpaCaseContainersConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("slpa_case_containers: invalid config: %w", err)
	}
	if cfg.RegistrationTemplateID == "" {
		return fmt.Errorf("slpa_case_containers: registration_template_id is required")
	}
	if ctx.Record == nil {
		return fmt.Errorf("slpa_case_containers: task record is nil")
	}
	if ctx.OutputNamespace == "" {
		return fmt.Errorf("slpa_case_containers: an output namespace is required")
	}

	var tasks []store.TaskRecord
	if ctx.Record.CaseID != "" {
		tasks = p.tasks.GetAllTasks(ctx.Context, ctx.Record.CaseID)
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = map[string]any{
		"containers": caseContainers(tasks, cfg),
	}
	return nil
}

// caseContainers pairs each registered dummy number with its real container.
func caseContainers(tasks []store.TaskRecord, cfg slpaCaseContainersConfig) []any {
	// Oldest first, so the containers keep the order they were registered in and
	// the latest pairing of a container is the one read last.
	slices.SortStableFunc(tasks, func(a, b store.TaskRecord) int {
		return a.UpdatedAt.Compare(b.UpdatedAt)
	})

	var dummies []string
	seen := map[string]bool{}
	paired := map[string]string{}
	for _, t := range tasks {
		switch {
		case t.TaskTemplateID == cfg.RegistrationTemplateID:
			form, _ := t.Data["userform"].(map[string]any)
			for _, row := range fields.Rows(form["containers"]) {
				dummy := strings.TrimSpace(fields.String(row, "containerNo"))
				if dummy != "" && !seen[dummy] {
					seen[dummy] = true
					dummies = append(dummies, dummy)
				}
			}
		case slices.Contains(cfg.PairingTemplateIDs, t.TaskTemplateID):
			dummy := strings.TrimSpace(fields.String(t.Data, "so_container_no"))
			if dummy != "" {
				paired[dummy] = pairedContainer(t.Data)
			}
		}
	}

	out := make([]any, 0, len(dummies))
	for _, dummy := range dummies {
		out = append(out, map[string]any{
			"container_no":      dummy,
			"real_container_no": paired[dummy],
		})
	}
	return out
}

// pairedContainer reads the real container a pairing task holds, or "" when it
// holds none: a consolidation not yet saved, or one the trader deleted. The
// consolidation records a delete as deleted, the gate pass as
// consolidation.deleted.
func pairedContainer(data map[string]any) string {
	consolidation, _ := data["consolidation"].(map[string]any)
	if deleted, _ := data["deleted"].(bool); deleted {
		return ""
	}
	if deleted, _ := consolidation["deleted"].(bool); deleted {
		return ""
	}
	return strings.TrimSpace(fields.String(data, "container_no"))
}
