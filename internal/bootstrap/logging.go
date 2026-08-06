package bootstrap

import (
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
	opts := &slog.HandlerOptions{
		AddSource: cfg.Server.Debug,
		Level:     cfg.Server.LogLevel,
	}
	logHandler := slog.NewTextHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(logging.NewHandler(logHandler)))
	httputil.CorrelationIDFunc = trace.GetTraceID
}
