// Package metrics records HTTP server request duration and serves it in
// Prometheus text format.
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Path is the unauthenticated scrape endpoint, registered like /health.
const Path = "/metrics"

const meterName = "github.com/OpenNSW/nsw-srilanka/internal/metrics"

// durationBuckets are the OpenTelemetry HTTP duration boundaries, in seconds.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

// Server records one histogram for every request except the scrape itself.
type Server struct {
	handler  http.Handler
	duration metric.Float64Histogram
	shutdown func(context.Context) error
}

// New starts a meter whose only series is http.server.request.duration.
func New() (*Server, error) {
	reg := prometheus.NewRegistry()
	exporter, err := otelprometheus.New(otelprometheus.WithRegisterer(reg))
	if err != nil {
		return nil, fmt.Errorf("create prometheus exporter: %w", err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	histogram, err := provider.Meter(meterName).Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests."),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		_ = provider.Shutdown(context.Background())
		return nil, fmt.Errorf("create request duration histogram: %w", err)
	}
	return &Server{
		handler:  promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		duration: histogram,
		shutdown: provider.Shutdown,
	}, nil
}

// Handler serves Prometheus text for this server's registry.
func (s *Server) Handler() http.Handler { return s.handler }

// Shutdown flushes and stops the meter provider.
func (s *Server) Shutdown(ctx context.Context) error { return s.shutdown(ctx) }

// Middleware records method, mux route pattern, and status code. The route
// label is the pattern (for example /api/v1/tasks/{id}), never the raw path.
// Requests to Path are served and not recorded.
func (s *Server) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == Path {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		s.duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", routeLabel(r.Pattern)),
			attribute.Int("http.response.status_code", status),
		))
	})
}

// routeLabel strips the method from a Go 1.22 ServeMux pattern. Unmatched
// requests share one label so arbitrary paths cannot raise cardinality.
func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if _, rest, ok := strings.Cut(pattern, " "); ok {
		return rest
	}
	return pattern
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
