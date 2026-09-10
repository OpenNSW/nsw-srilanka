package cusdec

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/core/trace/logging"
)

// captureLogs swaps the default logger for one writing to a buffer at the
// given level, and restores it afterwards.
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return buf
}

// The submission goes over TLS to a remote endpoint and no layer keeps the
// request body, so this log is the only record of what was sent. containerFlag
// is on it by name: the field was renamed in v1.6 and a submission carrying the
// old name is refused with nothing to show for it.
func TestBuildParts_LogsTheFieldsUnderReview(t *testing.T) {
	logs := captureLogs(t, slog.LevelInfo)
	form := minimalForm()
	form["transport"] = map[string]any{"containerized": true}
	form["containerCount"] = 4

	if _, err := (CusdecInterpreter{}).BuildParts(context.Background(), map[string]any{"payload": form}); err != nil {
		t.Fatalf("BuildParts: %v", err)
	}

	line := logs.String()
	for _, want := range []string{"cusdec: declaration built", `"container_flag":true`, `"number_of_containers":4`} {
		if !strings.Contains(line, want) {
			t.Errorf("log is missing %s\ngot: %s", want, line)
		}
	}
}

// The declaration is recorded in full at info. A rejection names a field, and
// answering it means seeing the value that field carried — which is worth the
// exporter and importer details travelling into wherever these logs are kept.
func TestBuildParts_LogsTheWholeDeclaration(t *testing.T) {
	logs := captureLogs(t, slog.LevelInfo)

	if _, err := (CusdecInterpreter{}).BuildParts(context.Background(), map[string]any{"payload": minimalForm()}); err != nil {
		t.Fatalf("BuildParts: %v", err)
	}

	var logged map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] != "cusdec: declaration payload" {
			continue
		}
		body, _ := entry["payload"].(string)
		if err := json.Unmarshal([]byte(body), &logged); err != nil {
			t.Fatalf("the logged payload is not the declaration JSON: %v", err)
		}
	}
	if logged == nil {
		t.Fatalf("no declaration payload was logged\ngot: %s", logs.String())
	}
	// What is logged is the submission itself, not a rendering of it: the same
	// object, reachable field by field.
	for _, key := range []string{"properties", "baseGeneralSegment", "generalSegment", "goodsShipments"} {
		if _, ok := logged[key]; !ok {
			t.Errorf("the logged declaration is missing %q", key)
		}
	}
}

// A form the mapping never delivered names the inputs that did arrive, so the
// workflow can be corrected without guessing.
func TestBuildParts_LogsTheInputsWhenTheFormIsMissing(t *testing.T) {
	logs := captureLogs(t, slog.LevelInfo)

	_, err := (CusdecInterpreter{}).BuildParts(context.Background(), map[string]any{"previous_edge_id": "e-1"})
	if err == nil {
		t.Fatal("expected a build failure")
	}
	if !strings.Contains(logs.String(), "previous_edge_id") {
		t.Errorf("the log does not name the inputs that arrived: %s", logs.String())
	}
}

// The point of taking a context is the traceId: a submission and the
// acknowledgement to it are one trace, so the two can be read together instead
// of being matched up by hand afterwards.
func TestSubmissionLogsCarryTheTraceID(t *testing.T) {
	logs := captureLogs(t, slog.LevelInfo)
	slog.SetDefault(slog.New(logging.NewHandler(slog.Default().Handler())))
	ctx := trace.ContextWithTraceID(context.Background(), "trace-abc123")

	if _, err := (CusdecInterpreter{}).BuildParts(ctx, map[string]any{"payload": minimalForm()}); err != nil {
		t.Fatalf("BuildParts: %v", err)
	}
	(CusdecInterpreter{}).InterpretContext(ctx, nil, map[string]any{
		"edgeId": "edge-1", "status": "RECEIVED",
	})

	var traced, untraced []string
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		msg, _ := entry["msg"].(string)
		if entry["traceId"] == "trace-abc123" {
			traced = append(traced, msg)
			continue
		}
		untraced = append(untraced, msg)
	}

	if len(untraced) > 0 {
		t.Errorf("these submission logs carry no traceId: %v", untraced)
	}
	// Both ends of the round-trip, on the one trace.
	for _, want := range []string{"cusdec: declaration built", "cusdec: submission accepted"} {
		found := false
		for _, msg := range traced {
			if msg == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is missing from the trace; got %v", want, traced)
		}
	}
}
