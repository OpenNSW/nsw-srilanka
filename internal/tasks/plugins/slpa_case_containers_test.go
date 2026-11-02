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

// fakeCaseTasks answers with the tasks it holds, and records the case asked for.
type fakeCaseTasks struct {
	tasks  []store.TaskRecord
	caseID string
}

func (f *fakeCaseTasks) GetAllTasks(_ context.Context, caseID string) []store.TaskRecord {
	f.caseID = caseID
	return f.tasks
}

const caseContainersConfig = `{
  "registration_template_id": "slpa-ecdn-submission-flow",
  "consolidation_template_id": "slpa-consolidation-flow",
  "gate_pass_template_id": "slpa-gate-pass-flow"
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

// consolidated is a consolidation pairing dummy with the real container number,
// as its resolve step records it: SLPA's ID for the container beside it.
func consolidated(minutes int, dummy, number string) store.TaskRecord {
	return task("slpa-consolidation-flow", minutes, map[string]any{
		"so_container_no": dummy, "container_no": number, "cap_sqid": "cap-" + number, "deleted": false,
	})
}

// deletedPairing is a gate pass from which the trader deleted the pairing with
// number. The gate pass names the container by SLPA's ID alone.
func deletedPairing(minutes int, number string) store.TaskRecord {
	return task("slpa-gate-pass-flow", minutes, map[string]any{
		"cap_id": "cap-" + number, "consolidation": map[string]any{"deleted": true},
	})
}

func TestSLPACaseContainers_PairsEachDummyWithTheContainerItWasConsolidatedTo(t *testing.T) {
	source, got := runCaseContainers(t,
		ecdnTask(0, "CBEX1E68944-20261102-01", "CBEX1E68944-20261102-02"),
		consolidated(10, "CBEX1E68944-20261102-01", "MSCU8492019"),
	)

	assert.Equal(t, "consignment-1", source.caseID, "the containers are the sundry order's own consignment's")
	assert.Equal(t, []any{
		container("CBEX1E68944-20261102-01", "MSCU8492019"),
		container("CBEX1E68944-20261102-02", ""),
	}, got, "a container not consolidated yet is listed by its dummy number alone")
}

// A pairing the trader deleted from the gate pass no longer pairs the dummy with
// anything, and the pairing made after it is the one that counts.
func TestSLPACaseContainers_ReadsTheLatestPairing(t *testing.T) {
	_, got := runCaseContainers(t,
		consolidated(30, "D-02", "TGHU1111111"),
		deletedPairing(20, "MSCU2222222"),
		consolidated(10, "D-02", "MSCU2222222"),
		consolidated(5, "D-01", "MSCU3333333"),
		deletedPairing(25, "MSCU3333333"),
		ecdnTask(0, "D-01", "D-02"),
	)

	assert.Equal(t, []any{container("D-01", ""), container("D-02", "TGHU1111111")}, got)
}

// Consolidating to the same container again after deleting it pairs it again.
func TestSLPACaseContainers_PairsAContainerConsolidatedAgainAfterADelete(t *testing.T) {
	_, got := runCaseContainers(t,
		ecdnTask(0, "D-01"),
		consolidated(10, "D-01", "MSCU8492019"),
		deletedPairing(20, "MSCU8492019"),
		consolidated(30, "D-01", "MSCU8492019"),
	)

	assert.Equal(t, []any{container("D-01", "MSCU8492019")}, got)
}

// A consolidation recorded before its resolve step names its dummy pairs nothing.
func TestSLPACaseContainers_IgnoresAConsolidationThatNamesNoDummy(t *testing.T) {
	_, got := runCaseContainers(t,
		ecdnTask(0, "D-01"),
		task("slpa-consolidation-flow", 10, map[string]any{"container_no": "MSCU8492019", "cap_sqid": "cap-MSCU8492019"}),
	)

	assert.Equal(t, []any{container("D-01", "")}, got)
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
