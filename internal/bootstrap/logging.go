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

const categoryAudit = "audit"

// ConfigureLogging installs the trace-aware slog handler as the process
// default and wires httputil's correlation ID hook to trace.GetTraceID, so
// the correlationId returned in error responses matches the traceId written
// to server logs. Must be called before any request is served.
//
// category=audit lines are omitted from the process (pod) log. They reach
// only an OTLP extra handler when bootstrap reconfigures logging after
// telemetry starts. Call logAuditExportStatus once that path is known.
func ConfigureLogging(cfg *config.Config) {
	configureLogging(cfg, os.Stdout)
}

// logAuditExportStatus reports when audit records will not be retained:
// server.logLevel above info drops Info audit lines on the export handler,
// and a missing OTLP log exporter means there is no destination at all.
// Messages are logged at the configured level so the filter they describe
// lets them through.
func logAuditExportStatus(cfg *config.Config, exportEnabled bool) {
	if cfg.Server.LogLevel > slog.LevelInfo {
		slog.Log(context.Background(), cfg.Server.LogLevel,
			"server.logLevel is above info: audit records (category=audit) are not exported")
		return
	}
	if !exportEnabled {
		slog.Warn("otel log export is off: audit records (category=audit) are not retained")
	}
}

// configureLogging is ConfigureLogging with an injectable log destination
// and optional extra slog handlers (e.g. OTLP) fan-out via MultiHandler.
// The process destination never receives category=audit; those records go
// only to extras (the OTLP handler when export is on).
func configureLogging(cfg *config.Config, dest io.Writer, extras ...slog.Handler) {
	opts := &slog.HandlerOptions{
		AddSource: cfg.Server.LogLevel <= slog.LevelDebug,
		Level:     cfg.Server.LogLevel,
	}
	// Drop audit on the pod stream so PII in audit metadata is not in
	// kubectl logs; export still receives them via extras.
	handlers := []slog.Handler{
		&dropCategory{category: categoryAudit, next: logging.NewHandler(slog.NewTextHandler(dest, opts))},
	}
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

// dropCategory omits records whose category attr matches category. Used so
// category=audit never reaches the process log while MultiHandler still
// forwards those records to an OTLP extra.
type dropCategory struct {
	category string
	drop     bool
	next     slog.Handler
}

func (h *dropCategory) Enabled(ctx context.Context, level slog.Level) bool {
	if h.drop {
		return false
	}
	return h.next.Enabled(ctx, level)
}

func (h *dropCategory) Handle(ctx context.Context, r slog.Record) error {
	if h.drop || recordHasCategory(r, h.category) {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *dropCategory) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &dropCategory{
		category: h.category,
		drop:     h.drop || attrsHaveCategory(attrs, h.category),
		next:     h.next.WithAttrs(attrs),
	}
}

func (h *dropCategory) WithGroup(name string) slog.Handler {
	return &dropCategory{category: h.category, drop: h.drop, next: h.next.WithGroup(name)}
}

func attrsHaveCategory(attrs []slog.Attr, category string) bool {
	for _, a := range attrs {
		if a.Key == "category" && a.Value.String() == category {
			return true
		}
	}
	return false
}

func recordHasCategory(r slog.Record, category string) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "category" && a.Value.String() == category {
			found = true
			return false
		}
		return true
	})
	return found
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
