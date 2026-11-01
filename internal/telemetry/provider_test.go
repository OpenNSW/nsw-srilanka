package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/core/trace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestStartNoopWithoutEndpoint(t *testing.T) {
	p, err := Start(t.Context(), Config{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := p.Shutdown(t.Context()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := p.Handler("tnsw-api", mux)
	if h != mux {
		t.Fatal("expected Handler to return next unchanged when endpoint is unset")
	}
	if p.SlogHandler() != nil {
		t.Fatal("expected SlogHandler nil when endpoint is unset")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestHandlerRecordsRoutePattern(t *testing.T) {
	p := testProvider(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tasks/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := p.Handler("tnsw-api", mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/abc-123", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	attrs := durationAttributes(t, collect(t, p))
	if got := attrString(attrs, semconv.HTTPRouteKey); got != "/api/v1/tasks/{id}" {
		t.Fatalf("http.route = %q, want /api/v1/tasks/{id}", got)
	}
	if got := attrString(attrs, semconv.HTTPRequestMethodKey); got != http.MethodPost {
		t.Fatalf("http.request.method = %q, want POST", got)
	}
	if got := attrInt(attrs, semconv.HTTPResponseStatusCodeKey); got != http.StatusCreated {
		t.Fatalf("http.response.status_code = %d, want 201", got)
	}
	for _, a := range attrs {
		if a.Key == semconv.HTTPRouteKey && a.Value.AsString() == "/api/v1/tasks/abc-123" {
			t.Fatal("raw path leaked into http.route")
		}
	}
}

func TestHandlerDropsServerAddressFromHost(t *testing.T) {
	p := testProvider(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := p.Handler("tnsw-api", mux)

	for _, host := range []string{"a.example", "b.example:8443", "c.example:443"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("host %q: status = %d, want 200", host, rec.Code)
		}
	}

	points := durationDataPoints(t, collect(t, p))
	if len(points) != 1 {
		t.Fatalf("got %d series, want 1 (server.address/port must not split by Host)", len(points))
	}
	for _, a := range points[0].Attributes.ToSlice() {
		if a.Key == semconv.ServerAddressKey || a.Key == semconv.ServerPortKey {
			t.Fatalf("unexpected attribute %s=%v", a.Key, a.Value.AsInterface())
		}
	}
}

func TestHandlerUsesCorrelationIDDespiteTraceparent(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithIDGenerator(correlationIDGenerator{}),
	)
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	p := &Provider{
		mp:       sdkmetric.NewMeterProvider(),
		tp:       tp,
		enabled:  true,
		shutdown: func(context.Context) error { return nil },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := p.Handler("tnsw-api", mux)

	const (
		correlationID = "ba06a9e9121683fe4b08a16cc00a3f59"
		remoteTraceID = "11111111111111111111111111111111"
		remoteSpanID  = "2222222222222222"
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	// sampled=0 and a different trace id. A public endpoint must still record
	// a root span under the correlation id, and only link the incoming span.
	// The default propagator ignores traceparent, so extract it here.
	req.Header.Set("traceparent", "00-"+remoteTraceID+"-"+remoteSpanID+"-00")
	ctx := propagation.TraceContext{}.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
	req = req.WithContext(trace.ContextWithTraceID(ctx, correlationID))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if got := spans[0].SpanContext.TraceID().String(); got != correlationID {
		t.Fatalf("trace id = %s, want %s", got, correlationID)
	}
	if spans[0].Parent.IsValid() {
		t.Fatalf("parent = %s, want none", spans[0].Parent.TraceID())
	}
	if len(spans[0].Links) != 1 {
		t.Fatalf("links = %d, want 1", len(spans[0].Links))
	}
	link := spans[0].Links[0].SpanContext
	if got := link.TraceID().String(); got != remoteTraceID {
		t.Fatalf("link trace id = %s, want %s", got, remoteTraceID)
	}
	if got := link.SpanID().String(); got != remoteSpanID {
		t.Fatalf("link span id = %s, want %s", got, remoteSpanID)
	}
}

func TestHandlerHealthProbesKeepMetricsWithoutSpans(t *testing.T) {
	for _, tc := range []struct {
		name, method, path                  string
		healthStatus, wantStatus, wantSpans int
	}{
		{"get probe", http.MethodGet, "/health", 200, 200, 0},
		{"head probe", http.MethodHead, "/health", 200, 200, 0},
		{"query probe", http.MethodGet, "/health?ready=true", 200, 200, 0},
		{"failed probe", http.MethodGet, "/health", 503, 503, 0},
		{"other method", http.MethodPost, "/health", 200, 405, 1},
		{"other path", http.MethodGet, "/healthz", 200, 404, 1},
		{"trailing slash", http.MethodGet, "/health/", 200, 404, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProvider(t)
			exp := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
			t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
			p.tp = tp
			mux := http.NewServeMux()
			called := false
			mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.Header().Set("X-Health", "checked")
				w.WriteHeader(tc.healthStatus)
			})
			req := httptest.NewRequest(tc.method, tc.path, nil)
			// A sampled upstream parent must not turn a probe into an exported span.
			req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
			ctx := propagation.TraceContext{}.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
			rec := httptest.NewRecorder()
			p.Handler("tnsw-api", mux).ServeHTTP(rec, req.WithContext(ctx))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantSpans == 0 && (!called || rec.Header().Get("X-Health") != "checked") {
				t.Fatal("health handler was not preserved")
			}
			if got := len(exp.GetSpans()); got != tc.wantSpans {
				t.Fatalf("spans = %d, want %d", got, tc.wantSpans)
			}
			points := durationDataPoints(t, collect(t, p))
			if len(points) != 1 || points[0].Count != 1 {
				t.Fatalf("duration points = %+v, want one observation", points)
			}
			attrs := points[0].Attributes.ToSlice()
			if got := attrInt(attrs, semconv.HTTPResponseStatusCodeKey); got != tc.wantStatus {
				t.Fatalf("metric status = %d, want %d", got, tc.wantStatus)
			}
			if tc.wantSpans == 0 && attrString(attrs, semconv.HTTPRouteKey) != "/health" {
				t.Fatal("health metric lost its route label")
			}
		})
	}
}

func TestRouteFromPattern(t *testing.T) {
	if got := routeFromPattern("GET /api/v1/tasks/{id}"); got != "/api/v1/tasks/{id}" {
		t.Fatalf("routeFromPattern = %q", got)
	}
	if got := routeFromPattern(""); got != "" {
		t.Fatalf("routeFromPattern = %q", got)
	}
}

type testMeterProvider struct {
	*Provider
	reader *sdkmetric.ManualReader
}

func testProvider(t *testing.T) *testMeterProvider {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(httpServerAttributeView()),
	)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })
	return &testMeterProvider{
		Provider: &Provider{mp: mp, enabled: true, shutdown: mp.Shutdown},
		reader:   reader,
	}
}

func collect(t *testing.T, p *testMeterProvider) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := p.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	return rm
}

func durationAttributes(t *testing.T, rm metricdata.ResourceMetrics) []attribute.KeyValue {
	t.Helper()
	points := durationDataPoints(t, rm)
	return points[0].Attributes.ToSlice()
}

func durationDataPoints(t *testing.T, rm metricdata.ResourceMetrics) []metricdata.HistogramDataPoint[float64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("unexpected data type %T", m.Data)
			}
			if len(hist.DataPoints) == 0 {
				t.Fatal("no histogram points")
			}
			return hist.DataPoints
		}
	}
	t.Fatal("http.server.request.duration not found")
	return nil
}

func attrString(attrs []attribute.KeyValue, key attribute.Key) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value.AsString()
		}
	}
	return ""
}

func attrInt(attrs []attribute.KeyValue, key attribute.Key) int {
	for _, a := range attrs {
		if a.Key == key {
			return int(a.Value.AsInt64())
		}
	}
	return 0
}
