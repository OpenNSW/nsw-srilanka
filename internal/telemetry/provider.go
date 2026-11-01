package telemetry

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/trace"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Provider owns the OTLP meter, tracer and logger providers.
type Provider struct {
	mp          metric.MeterProvider
	tp          *sdktrace.TracerProvider
	lp          *sdklog.LoggerProvider
	serviceName string
	enabled     bool
	shutdown    func(context.Context) error
}

// Start builds OTLP exporters when cfg.Endpoint is set. With an empty
// endpoint, Handler and SlogHandler are no-ops so local runs need no backend.
func Start(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled() {
		return &Provider{
			shutdown: func(context.Context) error { return nil },
		}, nil
	}

	metricsURL, err := cfg.signalURL("metrics")
	if err != nil {
		return nil, err
	}
	tracesURL, err := cfg.signalURL("traces")
	if err != nil {
		return nil, err
	}
	logsURL, err := cfg.signalURL("logs")
	if err != nil {
		return nil, err
	}

	serviceName := strings.TrimSpace(cfg.ServiceName)
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	metricOpts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpointURL(metricsURL)}
	traceOpts := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(tracesURL)}
	logOpts := []otlploghttp.Option{otlploghttp.WithEndpointURL(logsURL)}
	if len(cfg.Headers) > 0 {
		metricOpts = append(metricOpts, otlpmetrichttp.WithHeaders(cfg.Headers))
		traceOpts = append(traceOpts, otlptracehttp.WithHeaders(cfg.Headers))
		logOpts = append(logOpts, otlploghttp.WithHeaders(cfg.Headers))
	}

	metricExp, err := otlpmetrichttp.New(ctx, metricOpts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}
	traceExp, err := otlptracehttp.New(ctx, traceOpts...)
	if err != nil {
		_ = metricExp.Shutdown(ctx)
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	logExp, err := otlploghttp.New(ctx, logOpts...)
	if err != nil {
		_ = metricExp.Shutdown(ctx)
		_ = traceExp.Shutdown(ctx)
		return nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
		sdkmetric.WithView(httpServerAttributeView()),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		// Server spans use the request correlation id as their trace id when
		// that id is a W3C trace id, so a log's traceId finds the span.
		sdktrace.WithIDGenerator(correlationIDGenerator{}),
	)
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return &Provider{
		mp:          mp,
		tp:          tp,
		lp:          lp,
		serviceName: serviceName,
		enabled:     true,
		shutdown: func(ctx context.Context) error {
			return errors.Join(mp.Shutdown(ctx), tp.Shutdown(ctx), lp.Shutdown(ctx))
		},
	}, nil
}

// Handler wraps next with otelhttp so mux routes record metrics and traces.
// GET/HEAD /health probes retain metrics but do not record server spans.
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
	// A public endpoint starts a new root span and only links an incoming
	// traceparent. Otherwise a caller can parent the span or set sampled=0
	// and drop it, and that trace id replaces the correlation id.
	// otelhttp v0.72 has WithPublicEndpointFn, not WithPublicEndpoint.
	opts := []otelhttp.Option{
		otelhttp.WithMeterProvider(p.mp),
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true }),
	}
	if p.tp != nil {
		opts = append(opts, otelhttp.WithTracerProvider(p.tp))
	}
	instrumented := otelhttp.NewHandler(tagged, operation, opts...)
	// A filter would remove metrics as well as traces. Override only the tracer
	// for probes so uptime/status metrics and route labels remain available.
	probeOpts := append(opts, otelhttp.WithTracerProvider(noop.NewTracerProvider()))
	probe := otelhttp.NewHandler(tagged, operation, probeOpts...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == "/health" {
			probe.ServeHTTP(w, r)
			return
		}
		instrumented.ServeHTTP(w, r)
	})
}

// correlationIDGenerator copies trace.GetTraceID onto new root spans. A
// non-hex correlation id cannot be a W3C trace id, so those spans stay random.
type correlationIDGenerator struct{}

func (correlationIDGenerator) NewIDs(ctx context.Context) (oteltrace.TraceID, oteltrace.SpanID) {
	var tid oteltrace.TraceID
	if parsed, err := oteltrace.TraceIDFromHex(trace.GetTraceID(ctx)); err == nil && parsed.IsValid() {
		tid = parsed
	} else {
		_, _ = rand.Read(tid[:])
	}
	var sid oteltrace.SpanID
	_, _ = rand.Read(sid[:])
	return tid, sid
}

func (correlationIDGenerator) NewSpanID(context.Context, oteltrace.TraceID) oteltrace.SpanID {
	var sid oteltrace.SpanID
	_, _ = rand.Read(sid[:])
	return sid
}

// SlogHandler returns an slog handler that exports records over OTLP, or nil
// when export is off.
func (p *Provider) SlogHandler() slog.Handler {
	if !p.enabled {
		return nil
	}
	return otelslog.NewHandler(p.serviceName, otelslog.WithLoggerProvider(p.lp))
}

// Shutdown flushes and stops all providers.
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
