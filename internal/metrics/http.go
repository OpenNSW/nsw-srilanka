// Package metrics instruments the HTTP server with OpenTelemetry and exports
// via OTLP/HTTP when metrics.endpoint is set in config.yaml.
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
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

// Start builds an OTLP meter provider when cfg.Endpoint is set. With an empty
// endpoint, Handler is a no-op wrap so local runs need no collector.
func Start(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled() {
		return &Provider{
			shutdown: func(context.Context) error { return nil },
		}, nil
	}

	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpointURL(strings.TrimSpace(cfg.Endpoint)),
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlpmetrichttp.WithHeaders(cfg.Headers))
	}
	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(strings.TrimSpace(cfg.ServiceName))),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, fmt.Errorf("create resource: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
		sdkmetric.WithView(httpServerAttributeView()),
	)
	return &Provider{
		mp:       mp,
		enabled:  true,
		shutdown: mp.Shutdown,
	}, nil
}

// Handler wraps next with otelhttp so every mux route records HTTP metrics.
// When Start found no endpoint, next is returned unchanged.
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

// httpServerAttributeView drops server.address and server.port from HTTP
// server metrics. otelhttp takes those from the request Host header, so a
// client can create a new series per value; they are constant per service.
func httpServerAttributeView() sdkmetric.View {
	return sdkmetric.NewView(
		sdkmetric.Instrument{Name: "http.server.*"},
		sdkmetric.Stream{AttributeFilter: attribute.NewDenyKeysFilter(
			semconv.ServerAddressKey, semconv.ServerPortKey,
		)},
	)
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
