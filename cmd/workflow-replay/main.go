// Command workflow-replay replays exported Temporal workflow histories against the workflow
// engine this module builds with: github.com/OpenNSW/core/workflow at the version go.mod pins.
//
// Workers replay an open workflow's history with whatever engine they run, and core doesn't
// gate engine changes with workflow.GetVersion. So before deploying a core upgrade, replay a
// sample of histories from the target environment against the new build. A history that fails
// here belongs to a workflow that would stop with a nondeterminism error once workers on the
// new build pick it up. Pick workflows that are still open and went through a loop
// (a resubmission, a "needs more info" round, a re-verify), since those exercise the most
// engine code.
//
// Export each history with the Temporal CLI, naming the file after the workflow ID, then replay
// them:
//
//	temporal workflow show --namespace <ns> --workflow-id <id> --output json > histories/<id>.json
//	go run ./cmd/workflow-replay histories/*.json
//
// It prints one line per history and exits 1 if any history fails to replay.
package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"
	sdkworkflow "go.temporal.io/sdk/workflow"

	workflow "github.com/OpenNSW/core/workflow"
)

// workflowType is the name core's worker registers GraphInterpreterWorkflow under, so it is the
// workflow type every history records. Child workflows (BATCH_SPLIT) use the same type.
const workflowType = "GraphInterpreterWorkflow"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: workflow-replay <history.json>...")
		os.Exit(2)
	}
	logger := sdklog.NewStructuredLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	os.Exit(run(os.Args[1:], os.Stdout, logger))
}

// run replays each history file and reports the result to out. It returns the exit code: 0 when
// every history replays, 1 otherwise.
func run(paths []string, out io.Writer, logger sdklog.Logger) int {
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(workflow.GraphInterpreterWorkflow, sdkworkflow.RegisterOptions{Name: workflowType})

	failed := 0
	for _, path := range paths {
		if err := replay(replayer, logger, path); err != nil {
			failed++
			_, _ = fmt.Fprintf(out, "FAIL %s: %v\n", path, err)
			continue
		}
		_, _ = fmt.Fprintf(out, "ok   %s\n", path)
	}
	_, _ = fmt.Fprintf(out, "%d of %d histories replayed\n", len(paths)-failed, len(paths))
	if failed > 0 {
		return 1
	}
	return 0
}

// replay replays one history file under its original workflow ID. The engine derives each
// task's activity ID from the workflow ID, so a replay under the replayer's default ID would
// schedule activities the history doesn't have.
func replay(replayer worker.WorkflowReplayer, logger sdklog.Logger, path string) error {
	//nolint:gosec // G703: reading the files the operator names is this command's job; it runs with their own permissions.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	history, err := client.HistoryFromJSON(f, client.HistoryJSONOptions{})
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	workflowID, err := originalWorkflowID(history, path)
	if err != nil {
		return err
	}
	return replayer.ReplayWorkflowHistoryWithOptions(logger, history, worker.ReplayWorkflowHistoryOptions{
		OriginalExecution: sdkworkflow.Execution{ID: workflowID},
	})
}

// originalWorkflowID is the workflow ID the history's start event records. A Temporal server
// that doesn't record it there leaves the file name, which the export command sets to the
// workflow ID.
func originalWorkflowID(history *historypb.History, path string) (string, error) {
	events := history.GetEvents()
	if len(events) == 0 {
		return "", errors.New("history has no events")
	}
	if id := events[0].GetWorkflowExecutionStartedEventAttributes().GetWorkflowId(); id != "" {
		return id, nil
	}
	if id := strings.TrimSuffix(filepath.Base(path), ".json"); id != "" {
		return id, nil
	}
	return "", errors.New("history records no workflow ID; name the file <workflow-id>.json")
}
