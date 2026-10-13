package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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
