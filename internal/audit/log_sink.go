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

// NewLogSink returns a LogSink. A nil logger uses slog.Default().
func NewLogSink(logger *slog.Logger) *LogSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogSink{logger: logger.With(slog.String("category", categoryAudit))}
}

// Write implements Sink.
func (s *LogSink) Write(ctx context.Context, r Record) {
	if s == nil || s.logger == nil {
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
	s.logger.InfoContext(ctx, "audit", attrs...)
}
