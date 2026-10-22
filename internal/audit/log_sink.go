package audit

import (
	"context"
	"log/slog"
	"sync/atomic"
)

const categoryAudit = "audit"

// processLog is the logger a nil-logger LogSink writes to. Bootstrap installs
// it from the same stdout and OTLP handlers as the process logger, without
// the server.logLevel filter: audit lines are Info, so WARN would drop every
// record. Resolving it at Write time also picks up the handler installed
// after OpenTelemetry starts, which replaces slog.Default.
var processLog atomic.Pointer[slog.Logger]

// SetProcessLogger installs the logger a nil-logger LogSink writes to.
// The logger should accept Info. A nil argument clears it.
func SetProcessLogger(logger *slog.Logger) {
	if logger == nil {
		processLog.Store(nil)
		return
	}
	processLog.Store(logger.With(slog.String("category", categoryAudit)))
}

// LogSink writes enriched audit records as structured slog lines with
// category=audit so they can be filtered from ops logs on the same stream.
type LogSink struct {
	logger *slog.Logger
}

// NewLogSink returns a LogSink. A nil logger resolves SetProcessLogger, then
// slog.Default, on each Write rather than at construction.
func NewLogSink(logger *slog.Logger) *LogSink {
	if logger != nil {
		logger = logger.With(slog.String("category", categoryAudit))
	}
	return &LogSink{logger: logger}
}

func (s *LogSink) active() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	if l := processLog.Load(); l != nil {
		return l
	}
	return slog.Default().With(slog.String("category", categoryAudit))
}

// Write implements Sink.
func (s *LogSink) Write(ctx context.Context, r Record) {
	if s == nil {
		return
	}
	logger := s.active()
	if logger == nil {
		return
	}
	attrs := []any{
		"eventType", string(r.EventType),
		"action", string(r.Action),
		"status", string(r.Status),
		"actorType", string(r.ActorType),
		"actorID", r.ActorID,
		"targetType", string(r.TargetType),
	}
	if r.TargetID != "" {
		attrs = append(attrs, "targetID", r.TargetID)
	}
	if r.TraceID != "" {
		attrs = append(attrs, "traceID", r.TraceID)
	}
	if len(r.Metadata) > 0 {
		attrs = append(attrs, "metadata", r.Metadata)
	}
	if len(r.Message) > 0 {
		attrs = append(attrs, "message", string(r.Message))
	}
	logger.InfoContext(ctx, "audit", attrs...)
}
