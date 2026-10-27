package audit

import (
	"context"
	"log/slog"
	"sync"
)

// Category is the slog attribute value on every LogSink record. Bootstrap and
// collectors filter on this string; keep a single definition here.
const Category = "audit"

// LogSink writes enriched audit records as structured slog lines with
// category=audit. A nil logger discards writes. Bootstrap sets an OTLP-only
// logger after telemetry starts so records never reach the process (pod) log.
type LogSink struct {
	mu     sync.RWMutex
	logger *slog.Logger
}

// NewLogSink returns a LogSink. A nil logger discards until SetLogger provides
// one (typically the OTLP handler after OpenTelemetry starts).
func NewLogSink(logger *slog.Logger) *LogSink {
	s := &LogSink{}
	s.SetLogger(logger)
	return s
}

// SetLogger replaces the destination. nil discards. The category attribute is
// applied here so callers do not need to set it.
func (s *LogSink) SetLogger(logger *slog.Logger) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if logger != nil {
		s.logger = logger.With(slog.String("category", Category))
	} else {
		s.logger = nil
	}
}

// Write implements Sink.
func (s *LogSink) Write(ctx context.Context, r Record) {
	if s == nil {
		return
	}
	s.mu.RLock()
	logger := s.logger
	s.mu.RUnlock()
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
