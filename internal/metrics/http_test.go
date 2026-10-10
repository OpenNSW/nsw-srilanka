package metrics

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
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Setenv("OTEL_METRICS_EXPORTER", "")

	p, err := Start(t.Context())
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
		t.Fatal("expected Handler to return next unchanged when OTLP is unset")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestStartNoopWhenMetricsExporterNone(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")

	p, err := Start(t.Context())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Shutdown(t.Context())

	next := http.NewServeMux()
	if p.Handler("tnsw-api", next) != next {
		t.Fatal("expected no-op Handler when OTEL_METRICS_EXPORTER=none")
	}
}

func TestHandlerRecordsRoutePattern(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	p := &Provider{mp: mp, enabled: true, shutdown: mp.Shutdown}
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

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	attrs := durationAttributes(t, rm)
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

func TestRouteFromPattern(t *testing.T) {
	if got := routeFromPattern("GET /api/v1/tasks/{id}"); got != "/api/v1/tasks/{id}" {
		t.Fatalf("routeFromPattern = %q", got)
	}
	if got := routeFromPattern(""); got != "" {
		t.Fatalf("routeFromPattern = %q", got)
	}
}

func durationAttributes(t *testing.T, rm metricdata.ResourceMetrics) []attribute.KeyValue {
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
			return hist.DataPoints[0].Attributes.ToSlice()
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
