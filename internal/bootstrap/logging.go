package bootstrap

import (
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
			handlers = append(handlers, h)
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
