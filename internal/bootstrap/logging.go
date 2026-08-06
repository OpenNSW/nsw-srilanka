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

// configureLogging is ConfigureLogging with an injectable log destination.
func configureLogging(cfg *config.Config, dest io.Writer) {
	opts := &slog.HandlerOptions{
		AddSource: cfg.Server.Debug,
		Level:     cfg.Server.LogLevel,
	}
	logHandler := slog.NewTextHandler(dest, opts)
	slog.SetDefault(slog.New(logging.NewHandler(logHandler)))
	httputil.CorrelationIDFunc = trace.GetTraceID
}
