package plugins

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCaseTasks answers with the tasks it holds, and records the filter asked with.
type fakeCaseTasks struct {
	tasks  []store.TaskRecord
	filter store.TaskFilter
}

func (f *fakeCaseTasks) GetTasks(_ context.Context, filter store.TaskFilter) []store.TaskRecord {
	f.filter = filter
	return f.tasks
}

const caseContainersConfig = `{
  "registration_template_id": "slpa-ecdn-submission-flow",
  "pairing_template_ids": ["slpa-consolidation-flow", "slpa-gate-pass-flow"]
}`

var caseStart = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// task is a task of the case, updated minutes after caseStart.
func task(templateID string, minutes int, data map[string]any) store.TaskRecord {
	return store.TaskRecord{TaskTemplateID: templateID, UpdatedAt: caseStart.Add(time.Duration(minutes) * time.Minute), Data: data}
}

// ecdnTask is the ECDN task, listing each container by its dummy number.
func ecdnTask(minutes int, dummies ...string) store.TaskRecord {
	rows := make([]any, 0, len(dummies))
	for _, d := range dummies {
		rows = append(rows, map[string]any{"containerNo": d, "containerMark": "MARK"})
	}
	return task("slpa-ecdn-submission-flow", minutes, map[string]any{"userform": map[string]any{"containers": rows}})
}

func runCaseContainers(t *testing.T, tasks ...store.TaskRecord) (*fakeCaseTasks, []any) {
	t.Helper()
	source := &fakeCaseTasks{tasks: tasks}
	ctx := plugins.PluginContext{
		Context:         context.Background(),
		Record:          &store.TaskRecord{TaskID: "sundry-1", CaseID: "consignment-1"},
		OutputNamespace: "containers",
	}

	require.NoError(t, NewSLPACaseContainersPlugin(source).Execute(ctx, json.RawMessage(caseContainersConfig)))

	out, ok := ctx.Record.Data["containers"].(map[string]any)
	require.True(t, ok)
	containers, ok := out["containers"].([]any)
	require.True(t, ok)
	return source, containers
}

func container(dummy, realNo string) map[string]any {
	return map[string]any{"container_no": dummy, "real_container_no": realNo}
}

func TestSLPACaseContainers_PairsEachDummyWithTheContainerItWasConsolidatedTo(t *testing.T) {
	source, got := runCaseContainers(t,
		ecdnTask(0, "CBEX1E68944-20261102-01", "CBEX1E68944-20261102-02"),
		task("slpa-consolidation-flow", 10, map[string]any{"so_container_no": "CBEX1E68944-20261102-01", "container_no": "MSCU8492019", "deleted": false}),
	)

	// Only the case is set: every task of the consignment is read, whatever its read roles.
	assert.Equal(t, store.TaskFilter{CaseID: "consignment-1"}, source.filter, "the containers are the sundry order's own consignment's")
	assert.Equal(t, []any{
		container("CBEX1E68944-20261102-01", "MSCU8492019"),
		container("CBEX1E68944-20261102-02", ""),
	}, got, "a container not consolidated yet is listed by its dummy number alone")
}

// A consolidation the trader deleted no longer pairs the dummy with anything,
// whether the delete was recorded by the consolidation or the gate pass, and the
// pairing made after it is the one that counts.
func TestSLPACaseContainers_ReadsTheLatestPairing(t *testing.T) {
	_, got := runCaseContainers(t,
		task("slpa-consolidation-flow", 30, map[string]any{"so_container_no": "D-02", "container_no": "TGHU1111111"}),
		task("slpa-gate-pass-flow", 20, map[string]any{"so_container_no": "D-02", "container_no": "MSCU2222222",
			"consolidation": map[string]any{"deleted": true}}),
		task("slpa-consolidation-flow", 10, map[string]any{"so_container_no": "D-02", "container_no": "MSCU2222222"}),
		task("slpa-gate-pass-flow", 25, map[string]any{"so_container_no": "D-01", "container_no": "MSCU3333333",
			"consolidation": map[string]any{"deleted": true}}),
		ecdnTask(0, "D-01", "D-02"),
	)

	assert.Equal(t, []any{container("D-01", ""), container("D-02", "TGHU1111111")}, got)
}

func TestSLPACaseContainers_ListsEachDummyOnce(t *testing.T) {
	_, got := runCaseContainers(t, ecdnTask(0, "D-01", " D-01 ", ""), ecdnTask(5, "D-02", "D-01"))

	assert.Equal(t, []any{container("D-01", ""), container("D-02", "")}, got)
}

// Nothing registered yet still opens the form: the trader types the number.
func TestSLPACaseContainers_RecordsAnEmptyListForACaseWithNoContainers(t *testing.T) {
	_, got := runCaseContainers(t, task("slpa-sundry-order-flow", 0, map[string]any{}))

	assert.Empty(t, got)
}

func TestSLPACaseContainers_RequiresItsConfig(t *testing.T) {
	p := NewSLPACaseContainersPlugin(&fakeCaseTasks{})
	ctx := plugins.PluginContext{Context: context.Background(), Record: &store.TaskRecord{}, OutputNamespace: "containers"}

	require.ErrorContains(t, p.Execute(ctx, json.RawMessage(`{}`)), "registration_template_id")

	ctx.OutputNamespace = ""
	require.ErrorContains(t, p.Execute(ctx, json.RawMessage(caseContainersConfig)), "output namespace")
}
