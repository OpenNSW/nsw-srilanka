// Package metrics instruments the HTTP server with OpenTelemetry and exports
// via OTLP when configured through the standard OTEL_* environment variables.
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Provider owns the meter provider used by Handler.
type Provider struct {
	mp       metric.MeterProvider
	enabled  bool
	shutdown func(context.Context) error
}

// Start builds an OTLP meter provider when an OTLP endpoint is set. With no
// endpoint (and when OTEL_METRICS_EXPORTER=none), Handler is a no-op wrap so
// local runs need no collector.
func Start(ctx context.Context) (*Provider, error) {
	if !exportConfigured() {
		return &Provider{
			shutdown: func(context.Context) error { return nil },
		}, nil
	}

	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, fmt.Errorf("create resource: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)
	return &Provider{
		mp:       mp,
		enabled:  true,
		shutdown: mp.Shutdown,
	}, nil
}

// Handler wraps next with otelhttp so every mux route records HTTP metrics.
// When Start found no OTLP endpoint, next is returned unchanged.
func (p *Provider) Handler(operation string, next http.Handler) http.Handler {
	if !p.enabled {
		return next
	}
	// otelhttp does not set metrics http.route from ServeMux Pattern; tag it via
	// Labeler after next runs (Pattern is set by then, before otelhttp records).
	tagged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if route := routeFromPattern(r.Pattern); route != "" {
			if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
				labeler.Add(semconv.HTTPRoute(route))
			}
		}
	})
	return otelhttp.NewHandler(tagged, operation, otelhttp.WithMeterProvider(p.mp))
}

// Shutdown flushes and stops the meter provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.shutdown(ctx)
}

func exportConfigured() bool {
	if os.Getenv("OTEL_METRICS_EXPORTER") == "none" {
		return false
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != ""
}

// routeFromPattern strips the method from a Go ServeMux pattern.
func routeFromPattern(pattern string) string {
	if pattern == "" {
		return ""
	}
	if idx := strings.IndexByte(pattern, '/'); idx >= 0 {
		return pattern[idx:]
	}
	return ""
}
