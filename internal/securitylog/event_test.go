package securitylog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestEventWritesCategorySecurity(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	Event(context.Background(), logger, "http denied",
		"method", "GET",
		"path", "/api/v1/tasks/x",
		"callerKind", "anonymous",
		"callerID", "",
		"status", 401,
		"reason", "unauthenticated",
	)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if line["category"] != Category {
		t.Fatalf("category = %v, want %q", line["category"], Category)
	}
	if line["msg"] != "http denied" {
		t.Fatalf("msg = %v", line["msg"])
	}
	if line["reason"] != "unauthenticated" {
		t.Fatalf("reason = %v", line["reason"])
	}
	if line["level"] != "WARN" {
		t.Fatalf("level = %v, want WARN", line["level"])
	}
}
