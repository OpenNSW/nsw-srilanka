// Package metrics records HTTP server request duration and serves it in
// Prometheus text format.
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Path is the unauthenticated scrape endpoint, registered like /health.
const Path = "/metrics"

// durationBuckets match the usual HTTP latency boundaries, in seconds.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

// Server records one histogram for every request except the scrape itself.
type Server struct {
	handler  http.Handler
	duration *prometheus.HistogramVec
}

// New builds a registry whose only series is http_server_request_duration_seconds.
func New() *Server {
	reg := prometheus.NewRegistry()
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "Duration of HTTP server requests.",
		Buckets: durationBuckets,
	}, []string{"http_request_method", "http_route", "http_response_status_code"})
	reg.MustRegister(duration)
	return &Server{
		handler:  promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		duration: duration,
	}
}

// Handler serves Prometheus text for this server's registry.
func (s *Server) Handler() http.Handler { return s.handler }

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
		s.duration.WithLabelValues(r.Method, routeLabel(r.Pattern), strconv.Itoa(status)).
			Observe(time.Since(start).Seconds())
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
