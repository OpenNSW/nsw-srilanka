package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	sdklog "go.temporal.io/sdk/log"
)

// fixture is the history of one synthetic run of core's engine (START -> TASK -> END, the task
// completing synchronously), recorded on a local Temporal server and exported with
// `temporal workflow show --output json`. The worker identity and sticky task queue name, which
// named the machine, are replaced.
const fixture = "testdata/workflow-replay-fixture.json"

func discardLogger() sdklog.Logger {
	return sdklog.NewStructuredLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// writeVariant writes the fixture to dir/name after edit changes its start event's attributes.
func writeVariant(t *testing.T, name string, edit func(started map[string]any)) string {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	require.NoError(t, err)
	var history map[string]any
	require.NoError(t, json.Unmarshal(raw, &history))
	first := history["events"].([]any)[0].(map[string]any)
	edit(first["workflowExecutionStartedEventAttributes"].(map[string]any))

	path := filepath.Join(t.TempDir(), name)
	out, err := json.Marshal(history)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o600))
	return path
}

// The recorded history replays against the engine this module builds with. This only passes
// when the replay runs under the recorded workflow ID: the engine derives the task's activity ID
// from it.
func TestRunReplaysAHistory(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{fixture}, &out, discardLogger())

	require.Equal(t, 0, code, out.String())
	require.Contains(t, out.String(), "ok   "+fixture)
	require.Contains(t, out.String(), "1 of 1 histories replayed")
}

// A history the engine would replay differently is reported, and the exit code says so. Under
// another workflow ID the engine schedules the task under an activity ID the history doesn't
// have, which is the nondeterminism a deploy would hit.
func TestRunReportsAHistoryThatDoesNotReplay(t *testing.T) {
	other := writeVariant(t, "other.json", func(started map[string]any) { started["workflowId"] = "another-workflow" })
	var out bytes.Buffer

	code := run([]string{fixture, other}, &out, discardLogger())

	require.Equal(t, 1, code)
	require.Contains(t, out.String(), "ok   "+fixture)
	require.Contains(t, out.String(), "FAIL "+other)
	require.Contains(t, out.String(), "1 of 2 histories replayed")
}

// A server that doesn't record the workflow ID in the start event leaves the file name, which the
// export command sets to the workflow ID.
func TestRunTakesTheWorkflowIDFromTheFileNameWhenTheHistoryLacksIt(t *testing.T) {
	noID := func(started map[string]any) { delete(started, "workflowId") }
	named := writeVariant(t, "workflow-replay-fixture.json", noID)
	misnamed := writeVariant(t, "something-else.json", noID)
	var out bytes.Buffer

	code := run([]string{named, misnamed}, &out, discardLogger())

	require.Equal(t, 1, code)
	require.Contains(t, out.String(), "ok   "+named)
	require.Contains(t, out.String(), "FAIL "+misnamed)
}

// A file that can't be read as a history is reported like a failed replay rather than stopping
// the run.
func TestRunReportsAFileThatIsNotAHistory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	var out bytes.Buffer

	code := run([]string{missing, fixture}, &out, discardLogger())

	require.Equal(t, 1, code)
	require.Contains(t, out.String(), "FAIL "+missing)
	require.Contains(t, out.String(), "ok   "+fixture)
}
