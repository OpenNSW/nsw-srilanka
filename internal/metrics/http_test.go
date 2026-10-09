package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareRecordsRoutePatternAndStatus(t *testing.T) {
	srv := New()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tasks/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.Handle("GET "+Path, srv.Handler())
	h := srv.Middleware(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/abc-123", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	body := scrape(t, h)
	if strings.Contains(body, "abc-123") {
		t.Fatalf("raw path leaked into metrics:\n%s", body)
	}
	for _, want := range []string{
		"http_server_request_duration_seconds",
		`http_request_method="POST"`,
		`http_response_status_code="201"`,
		`http_route="/api/v1/tasks/{id}"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, `http_route="/metrics"`) {
		t.Fatalf("scrape was recorded:\n%s", body)
	}
}

func TestMiddlewareRecordsUnmatchedAs404(t *testing.T) {
	srv := New()
	mux := http.NewServeMux()
	mux.Handle("GET "+Path, srv.Handler())
	h := srv.Middleware(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no/such", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	body := scrape(t, h)
	for _, want := range []string{
		`http_request_method="GET"`,
		`http_response_status_code="404"`,
		`http_route="unmatched"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %s:\n%s", want, body)
		}
	}
}

func TestMiddlewareDefaultsUnwrittenStatusTo200(t *testing.T) {
	srv := New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /empty", func(http.ResponseWriter, *http.Request) {})
	mux.Handle("GET "+Path, srv.Handler())
	h := srv.Middleware(mux)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/empty", nil))

	body := scrape(t, h)
	if !strings.Contains(body, `http_response_status_code="200"`) || !strings.Contains(body, `http_route="/empty"`) {
		t.Fatalf("metrics missing default status:\n%s", body)
	}
}

func TestRouteLabel(t *testing.T) {
	if got := routeLabel("GET /api/v1/tasks/{id}"); got != "/api/v1/tasks/{id}" {
		t.Fatalf("routeLabel = %q", got)
	}
	if got := routeLabel(""); got != "unmatched" {
		t.Fatalf("routeLabel = %q", got)
	}
}

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape status = %d, body %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}
