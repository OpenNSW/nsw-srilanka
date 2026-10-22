package securitylog

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/core/trace/logging"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

func TestMiddlewareLogsCallerPayloadAndResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	const payload = `{"command":"approve","note":"ok"}`
	const response = `{"status":"accepted"}`

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := authn.ContextWithPrincipal(r.Context(), &authn.Principal{
			Kind:   authn.KindUser,
			UserID: "user-1",
		})
		if got, ok := authn.FromContext(ctx); !ok || got.Subject() != "user-1" {
			t.Fatalf("principal on handler context = %v ok=%v", got, ok)
		}
		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(got) != payload {
			t.Fatalf("handler body = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(response))
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/abc?x=1", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer super-secret-token")

	rec := httptest.NewRecorder()
	Middleware(logger)(mux).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated || rec.Body.String() != response {
		t.Fatalf("client response = %d %q", rec.Code, rec.Body.String())
	}

	line := decodeLine(t, buf.Bytes())
	if strings.Contains(buf.String(), "super-secret-token") {
		t.Fatal("security log included the Authorization header")
	}
	assertEqual(t, "category", line["category"], "security")
	assertEqual(t, "msg", line["msg"], "http request")
	assertEqual(t, "method", line["method"], "POST")
	assertEqual(t, "path", line["path"], "/api/v1/tasks/abc")
	assertEqual(t, "query", line["query"], "x=1")
	assertEqual(t, "pattern", line["pattern"], "POST /api/v1/tasks/{id}")
	assertEqual(t, "callerKind", line["callerKind"], "user")
	assertEqual(t, "callerID", line["callerID"], "user-1")
	assertEqual(t, "status", line["status"], float64(http.StatusCreated))
	assertEqual(t, "requestBody", line["requestBody"], payload)
	assertEqual(t, "responseBody", line["responseBody"], response)
	if _, ok := line["time"]; !ok {
		t.Fatal("expected slog timestamp")
	}
	if _, ok := line["durationMs"]; !ok {
		t.Fatal("expected durationMs")
	}
}

func TestMiddlewareAnonymousWhenUnauthenticated(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	const payload = `{"event":"ping"}`

	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authn.FromContext(r.Context()); ok {
			t.Fatal("expected no principal")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"authentication required"}`)
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slpa", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)

	line := decodeLine(t, buf.Bytes())
	assertEqual(t, "callerKind", line["callerKind"], "anonymous")
	assertEqual(t, "callerID", line["callerID"], "")
	assertEqual(t, "status", line["status"], float64(http.StatusUnauthorized))
	assertEqual(t, "requestBody", line["requestBody"], payload)
	assertEqual(t, "responseBody", line["responseBody"], `{"error":"authentication required"}`)
}

func TestMiddlewareOmitsNonTextBodies(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	body := []byte{0xff, 0xfe, 0x00, 0x01}

	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !bytes.Equal(got, body) {
			t.Fatalf("handler body = %v", got)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/storage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	h.ServeHTTP(httptest.NewRecorder(), req)

	line := decodeLine(t, buf.Bytes())
	if _, ok := line["requestBody"]; ok {
		t.Fatalf("logged binary request body: %v", line["requestBody"])
	}
	if _, ok := line["responseBody"]; ok {
		t.Fatalf("logged binary response body: %v", line["responseBody"])
	}
	assertEqual(t, "requestBodyOmitted", line["requestBodyOmitted"], true)
	assertEqual(t, "responseBodyOmitted", line["responseBodyOmitted"], true)
	assertEqual(t, "responseBytes", line["responseBytes"], float64(len(body)))
}

func TestMiddlewareTruncatesAndStillFeedsTheHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	payload := strings.Repeat("a", maxLoggedBodyBytes+32)

	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(got) != payload {
			t.Fatalf("handler saw %d bytes, want %d", len(got), len(payload))
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, strings.Repeat("b", maxLoggedBodyBytes+8))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/abc/steps/s", strings.NewReader(payload))
	req.Header.Set("Content-Type", "text/plain")
	h.ServeHTTP(httptest.NewRecorder(), req)

	line := decodeLine(t, buf.Bytes())
	logged, _ := line["requestBody"].(string)
	if len(logged) != maxLoggedBodyBytes {
		t.Fatalf("logged request body length = %d, want %d", len(logged), maxLoggedBodyBytes)
	}
	assertEqual(t, "requestBodyTruncated", line["requestBodyTruncated"], true)
	assertEqual(t, "responseBodyTruncated", line["responseBodyTruncated"], true)
	assertEqual(t, "requestBytes", line["requestBytes"], float64(len(payload)))
}

func TestMiddlewareCarriesTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logging.NewHandler(slog.NewJSONHandler(&buf, nil)))

	h := trace.TraceMiddleware(Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Trace-ID", "trace-abc")
	h.ServeHTTP(httptest.NewRecorder(), req)

	line := decodeLine(t, buf.Bytes())
	assertEqual(t, "traceId", line["traceId"], "trace-abc")
	assertEqual(t, "callerKind", line["callerKind"], "anonymous")
	assertEqual(t, "status", line["status"], float64(http.StatusNoContent))
}

func decodeLine(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &line); err != nil {
		t.Fatalf("decode log line %q: %v", raw, err)
	}
	return line
}

func assertEqual(t *testing.T, name string, got, want any) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
}
