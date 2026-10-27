package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/core/trace/logging"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

// ConfigureLogging installs the trace-aware slog handler as the process
// default and wires httputil's correlation ID hook to trace.GetTraceID, so
// the correlationId returned in error responses matches the traceId written
// to server logs. Must be called before any request is served.
//
// Audit records do not use this default logger. Bootstrap points LogSink at
// the OTLP handler when telemetry starts (before workers). Call
// logAuditExportStatus once that path is known.
func ConfigureLogging(cfg *config.Config) {
	configureLogging(cfg, os.Stdout)
}

// logAuditExportStatus reports when audit records will not be retained.
// Both conditions are reported when both apply: a high logLevel suppresses
// Info audit lines on the export handler, and a missing OTLP exporter means
// there is no destination. Messages are logged at least at warn, or at the
// configured level when that is higher, so the filter they describe lets
// them through.
func logAuditExportStatus(cfg *config.Config, exportEnabled bool) {
	if cfg.Server.LogLevel > slog.LevelInfo {
		slog.Log(context.Background(), cfg.Server.LogLevel,
			"server.logLevel is above info: audit records (category=audit) are not exported")
	}
	if !exportEnabled {
		level := slog.LevelWarn
		if cfg.Server.LogLevel > level {
			level = cfg.Server.LogLevel
		}
		slog.Log(context.Background(), level,
			"otel log export is off: audit records (category=audit) are not retained")
	}
}

// configureLogging is ConfigureLogging with an injectable log destination
// and optional extra slog handlers (e.g. OTLP) fan-out via MultiHandler.
// Audit does not go through this path; see NewLogSink in Build.
func configureLogging(cfg *config.Config, dest io.Writer, extras ...slog.Handler) {
	opts := &slog.HandlerOptions{
		AddSource: cfg.Server.LogLevel <= slog.LevelDebug,
		Level:     cfg.Server.LogLevel,
	}
	handlers := []slog.Handler{logging.NewHandler(slog.NewTextHandler(dest, opts))}
	for _, h := range extras {
		if h != nil {
			// otelslog reports every level as enabled. Without this filter,
			// MultiHandler.Enabled is true for DEBUG even when logLevel is
			// INFO, so those records are exported and debug-gated payload
			// logging turns on. logging.NewHandler attaches traceId so an
			// exported record can be found by the correlationId returned to
			// the client.
			handlers = append(handlers, &levelFilter{min: cfg.Server.LogLevel, next: logging.NewHandler(h)})
		}
	}
	var logHandler slog.Handler
	if len(handlers) == 1 {
		logHandler = handlers[0]
	} else {
		logHandler = slog.NewMultiHandler(handlers...)
	}
	slog.SetDefault(slog.New(logHandler))
	httputil.CorrelationIDFunc = trace.GetTraceID
}

// auditExportHandler wraps the OTLP slog handler so Info audit lines honor
// server.logLevel the same way the process MultiHandler extras do.
func auditExportHandler(cfg *config.Config, otlp slog.Handler) slog.Handler {
	if otlp == nil {
		return nil
	}
	return &levelFilter{min: cfg.Server.LogLevel, next: logging.NewHandler(otlp)}
}

// levelFilter drops records below min before they reach next. Enabled must
// agree with Handle so slog skips building a record the handler will ignore.
type levelFilter struct {
	min  slog.Level
	next slog.Handler
}

func (h *levelFilter) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.min && h.next.Enabled(ctx, level)
}

func (h *levelFilter) Handle(ctx context.Context, r slog.Record) error {
	if r.Level < h.min {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *levelFilter) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelFilter{min: h.min, next: h.next.WithAttrs(attrs)}
}

func (h *levelFilter) WithGroup(name string) slog.Handler {
	return &levelFilter{min: h.min, next: h.next.WithGroup(name)}
}
