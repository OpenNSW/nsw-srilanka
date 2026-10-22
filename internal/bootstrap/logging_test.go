package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

func TestErrorResponseCorrelationIDMatchesTraceID(t *testing.T) {
	// The correlationId returned to a client must equal the traceId in server
	// logs, or a client-reported ID can't be looked up. See PR #276.
	prevDefault := slog.Default()
	prevCorrelationIDFunc := httputil.CorrelationIDFunc
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		httputil.CorrelationIDFunc = prevCorrelationIDFunc
	})

	var logOutput bytes.Buffer
	configureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelInfo}}, &logOutput)

	var got httputil.ErrorResponse
	var traceID string

	h := trace.TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID = trace.GetTraceID(r.Context())
		slog.InfoContext(r.Context(), "handling request")
		httputil.Error(w, r, http.StatusNotFound, "not found")
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))

	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if traceID == "" {
		t.Fatal("middleware did not set a trace ID")
	}
	if got.CorrelationID != traceID {
		t.Fatalf("correlationId %q does not match traceId %q", got.CorrelationID, traceID)
	}
	if !strings.Contains(logOutput.String(), "traceId="+traceID) {
		t.Fatalf("expected server log to contain traceId=%s, got: %s", traceID, logOutput.String())
	}
}

func TestConfigureLoggingAddSourceTiedToDebugLevel(t *testing.T) {
	prevDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prevDefault) })

	var debugOutput bytes.Buffer
	configureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelDebug}}, &debugOutput)
	slog.Info("test message")
	if !strings.Contains(debugOutput.String(), "source=") {
		t.Errorf("expected source location in log output at debug level, got: %s", debugOutput.String())
	}

	var infoOutput bytes.Buffer
	configureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelInfo}}, &infoOutput)
	slog.Info("test message")
	if strings.Contains(infoOutput.String(), "source=") {
		t.Errorf("expected no source location in log output at info level, got: %s", infoOutput.String())
	}
}

// alwaysEnabled records every Handle. It reports every level as enabled, the
// same way otelslog does, so a missing level filter would accept DEBUG.
type alwaysEnabled struct {
	levels []slog.Level
	attrs  []map[string]string
}

func (h *alwaysEnabled) Enabled(context.Context, slog.Level) bool { return true }

func (h *alwaysEnabled) Handle(_ context.Context, r slog.Record) error {
	h.levels = append(h.levels, r.Level)
	got := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		got[a.Key] = a.Value.String()
		return true
	})
	h.attrs = append(h.attrs, got)
	return nil
}

func (h *alwaysEnabled) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *alwaysEnabled) WithGroup(string) slog.Handler { return h }

func TestConfigureLoggingExtrasHonorLogLevel(t *testing.T) {
	prevDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prevDefault) })

	extra := &alwaysEnabled{}
	configureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelInfo}}, io.Discard, extra)

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("debug enabled at info log level")
	}
	slog.Debug("hidden")
	slog.Info("shown")
	if len(extra.levels) != 1 || extra.levels[0] != slog.LevelInfo {
		t.Fatalf("extra handler levels = %v, want [INFO]", extra.levels)
	}
}

func TestConfigureLoggingAuditIgnoresLevelAndUsesLatestDest(t *testing.T) {
	prevDefault := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		nswaudit.SetProcessLogger(nil)
	})

	var first bytes.Buffer
	configureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelWarn}}, &first)
	sink := nswaudit.NewLogSink(nil)

	var second bytes.Buffer
	extra := &alwaysEnabled{}
	configureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelWarn}}, &second, extra)
	slog.Info("ops info hidden")
	sink.Write(context.Background(), nswaudit.Record{
		EventType:  nswaudit.EventTask,
		Action:     nswaudit.ActionUpdate,
		Status:     nswaudit.StatusSuccess,
		ActorType:  nswaudit.ActorMember,
		ActorID:    "u1",
		TargetType: nswaudit.TargetTask,
		TargetID:   "t1",
	})

	if strings.Contains(first.String(), "category=audit") {
		t.Fatalf("audit line used the logger from before reconfigure: %s", first.String())
	}
	if !strings.Contains(second.String(), "category=audit") {
		t.Fatalf("audit line missing from the latest destination: %s", second.String())
	}
	if strings.Contains(second.String(), "ops info hidden") {
		t.Fatalf("info ops log written at warn level: %s", second.String())
	}
	var auditRecords int
	for _, level := range extra.levels {
		if level == slog.LevelInfo {
			auditRecords++
		}
	}
	if auditRecords != 1 {
		t.Fatalf("extra handler info records = %d, want 1 (the audit line)", auditRecords)
	}
}

func TestConfigureLoggingExtrasIncludeTraceID(t *testing.T) {
	prevDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prevDefault) })

	extra := &alwaysEnabled{}
	configureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelInfo}}, io.Discard, extra)

	const traceID = "abc123"
	ctx := trace.ContextWithTraceID(context.Background(), traceID)
	slog.InfoContext(ctx, "handled")

	if len(extra.attrs) != 1 {
		t.Fatalf("extra records = %d, want 1", len(extra.attrs))
	}
	if extra.attrs[0]["traceId"] != traceID {
		t.Fatalf("traceId = %q, want %q", extra.attrs[0]["traceId"], traceID)
	}
}
