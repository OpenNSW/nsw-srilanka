package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

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
	// ConsolidationTemplateID is the task that pairs a dummy number with a real
	// container: it records both, and SLPA's ID for the real one.
	ConsolidationTemplateID string `json:"consolidation_template_id"`
	// GatePassTemplateID is the task a trader deletes a pairing from. It records
	// the delete against SLPA's ID for the real container.
	GatePassTemplateID string `json:"gate_pass_template_id"`
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

// pairing is a real container a dummy number was consolidated to, and when.
type pairing struct {
	sqid, number string
	at           time.Time
}

// caseContainers pairs each registered dummy number with its real container.
//
// A dummy's pairing is its latest consolidation, unless a gate pass deleted that
// real container after it. The gate pass does not record the dummy, so a delete
// is matched to a pairing by SLPA's ID for the real container.
func caseContainers(tasks []store.TaskRecord, cfg slpaCaseContainersConfig) []any {
	// Oldest first, so the containers keep the order they were registered in and
	// the latest pairing of a container is the one read last.
	slices.SortStableFunc(tasks, func(a, b store.TaskRecord) int {
		return a.UpdatedAt.Compare(b.UpdatedAt)
	})

	var dummies []string
	seen := map[string]bool{}
	paired := map[string]pairing{}
	deletedAt := map[string]time.Time{}
	for _, t := range tasks {
		switch t.TaskTemplateID {
		case cfg.RegistrationTemplateID:
			form, _ := t.Data["userform"].(map[string]any)
			for _, row := range fields.Rows(form["containers"]) {
				dummy := strings.TrimSpace(fields.String(row, "containerNo"))
				if dummy != "" && !seen[dummy] {
					seen[dummy] = true
					dummies = append(dummies, dummy)
				}
			}
		case cfg.ConsolidationTemplateID:
			dummy := strings.TrimSpace(fields.String(t.Data, "so_container_no"))
			number := strings.TrimSpace(fields.String(t.Data, "container_no"))
			if deleted, _ := t.Data["deleted"].(bool); dummy == "" || number == "" || deleted {
				continue
			}
			paired[dummy] = pairing{sqid: fields.String(t.Data, "cap_sqid"), number: number, at: t.UpdatedAt}
		case cfg.GatePassTemplateID:
			consolidation, _ := t.Data["consolidation"].(map[string]any)
			if deleted, _ := consolidation["deleted"].(bool); deleted {
				if sqid := fields.String(t.Data, "cap_id"); sqid != "" {
					deletedAt[sqid] = t.UpdatedAt
				}
			}
		}
	}

	out := make([]any, 0, len(dummies))
	for _, dummy := range dummies {
		realNo := ""
		if p, ok := paired[dummy]; ok {
			if at, deleted := deletedAt[p.sqid]; !deleted || at.Before(p.at) {
				realNo = p.number
			}
		}
		out = append(out, map[string]any{
			"container_no":      dummy,
			"real_container_no": realNo,
		})
	}
	return out
}
