package audit

import (
	"context"
	"log/slog"
)

const categoryAudit = "audit"

// LogSink writes enriched audit records as structured slog lines with
// category=audit so they can be filtered from ops logs on the same stream.
type LogSink struct {
	logger *slog.Logger
}

// NewLogSink returns a LogSink. A nil logger uses slog.Default on each Write
// rather than at construction, so it follows server.logLevel and the handler
// installed after OpenTelemetry starts, which replaces slog.Default.
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
