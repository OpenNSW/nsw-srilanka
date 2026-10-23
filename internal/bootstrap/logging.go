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
func ConfigureLogging(cfg *config.Config) {
	configureLogging(cfg, os.Stdout)
	// configureLogging runs again once OpenTelemetry is up, so the warning
	// stays here and is logged once.
	logSuppressedAudit(cfg)
}

// logSuppressedAudit reports that audit records are not written when
// server.logLevel is above info. Audit lines are Info, and the message is
// logged at the configured level so the filter it describes lets it through.
func logSuppressedAudit(cfg *config.Config) {
	if cfg.Server.LogLevel > slog.LevelInfo {
		slog.Log(context.Background(), cfg.Server.LogLevel,
			"server.logLevel is above info: audit records (category=audit) are not written")
	}
}

// configureLogging is ConfigureLogging with an injectable log destination
// and optional extra slog handlers (e.g. OTLP) fan-out via MultiHandler.
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
