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
		p := &authn.Principal{Kind: authn.KindUser, UserID: "user-1"}
		// attachPrincipal records the HTTP caller. ContextWithPrincipal only
		// puts the principal on the handler's context.
		if slot := authn.RequestCallerFrom(r.Context()); slot != nil {
			slot.Principal = p
		}
		ctx := authn.ContextWithPrincipal(r.Context(), p)
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
		// A webhook authenticates itself and then reads the body. The log
		// records what the handler actually read.
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatalf("read body: %v", err)
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

func TestMiddlewareChunkedBodyUsesBytesRead(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	payload := strings.Repeat("a", maxLoggedBodyBytes+32)

	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if len(got) != len(payload) {
			t.Fatalf("handler saw %d bytes, want %d", len(got), len(payload))
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/abc", strings.NewReader(payload))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "text/plain")
	h.ServeHTTP(httptest.NewRecorder(), req)

	line := decodeLine(t, buf.Bytes())
	assertEqual(t, "requestBytes", line["requestBytes"], float64(len(payload)))
	assertEqual(t, "requestBodyTruncated", line["requestBodyTruncated"], true)
}

func TestMiddlewareOmitsChunkedBinaryWithSize(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	body := []byte{0xff, 0xfe, 0x00, 0x01}

	h := Middleware(logger)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/storage", bytes.NewReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")
	h.ServeHTTP(httptest.NewRecorder(), req)

	line := decodeLine(t, buf.Bytes())
	if _, ok := line["requestBody"]; ok {
		t.Fatalf("logged binary request body: %v", line["requestBody"])
	}
	assertEqual(t, "requestBodyOmitted", line["requestBodyOmitted"], true)
	assertEqual(t, "requestBytes", line["requestBytes"], float64(len(body)))
}

func TestMiddlewareIgnoresInterimStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	// ResponseRecorder keeps the first status and then refuses a body for 1xx.
	// This writer accepts the body after the final status, as net/http does.
	dst := &finalStatus{ResponseWriter: httptest.NewRecorder()}
	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusContinue)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	h.ServeHTTP(dst, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/abc", nil))

	line := decodeLine(t, buf.Bytes())
	assertEqual(t, "status", line["status"], float64(http.StatusCreated))
	assertEqual(t, "responseBody", line["responseBody"], `{"ok":true}`)
}

func TestMiddlewarePassesReadFromThrough(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	const file = "file-bytes-not-for-the-log"
	dst := &sendFile{ResponseWriter: httptest.NewRecorder()}

	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		// A reader without WriteTo makes io.Copy use the writer's ReadFrom.
		n, err := io.Copy(w, plainReader{strings.NewReader(file)})
		if err != nil || n != int64(len(file)) {
			t.Fatalf("copy = %d, %v", n, err)
		}
	}))
	h.ServeHTTP(dst, httptest.NewRequest(http.MethodGet, "/api/v1/storage/k", nil))

	if !dst.used {
		t.Fatal("download did not use ReadFrom")
	}
	if dst.buf.String() != file {
		t.Fatalf("client got %q", dst.buf.String())
	}
	raw := buf.String()
	if strings.Contains(raw, file) {
		t.Fatal("security log copied the downloaded file")
	}
	line := decodeLine(t, []byte(raw))
	assertEqual(t, "responseBodyOmitted", line["responseBodyOmitted"], true)
	assertEqual(t, "responseBytes", line["responseBytes"], float64(len(file)))
}

func TestMiddlewareLogsWhenHandlerPanics(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Middleware(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/tasks/abc", nil))
	}()
	if !panicked {
		t.Fatal("panic was swallowed")
	}

	line := decodeLine(t, buf.Bytes())
	assertEqual(t, "path", line["path"], "/api/v1/tasks/abc")
	assertEqual(t, "callerKind", line["callerKind"], "anonymous")
	assertEqual(t, "status", line["status"], float64(http.StatusInternalServerError))
}

func TestMiddlewareSkipsHealth(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	called := false
	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if !called {
		t.Fatal("health handler was not called")
	}
	if buf.Len() != 0 {
		t.Fatalf("health was logged: %s", buf.String())
	}
}

func TestMiddlewareDoesNotReadBodyBeforeHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	body := &countReader{r: strings.NewReader(`{"note":"secret payload"}`)}

	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if body.n != 0 {
			t.Fatalf("body read before the handler decided, %d bytes", body.n)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/abc", body)
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if body.n != 0 {
		t.Fatalf("middleware read %d bytes the handler never asked for", body.n)
	}
	line := decodeLine(t, buf.Bytes())
	if _, ok := line["requestBody"]; ok {
		t.Fatalf("logged a body the handler did not read: %v", line["requestBody"])
	}
}

func TestMiddlewareRedactsSignedURLsAndCallbackToken(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	const token = "callback-secret"
	const upload = "https://s3.example/bucket/k?X-Amz-Signature=sig-secret"
	const download = "https://s3.example/bucket/k?X-Amz-Credential=cred-secret"

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/callbacks/{token}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"download_url":"`+download+`","expires_at":1}`)
	})

	payload := `{"filename":"a.pdf","upload_url":"` + upload + `","note":"keep me"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/"+token+"?token="+token+"&x=1", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	Middleware(logger)(mux).ServeHTTP(httptest.NewRecorder(), req)

	raw := buf.String()
	for _, secret := range []string{token, "sig-secret", "cred-secret", upload, download} {
		if strings.Contains(raw, secret) {
			t.Fatalf("security log contained %q: %s", secret, raw)
		}
	}
	line := decodeLine(t, []byte(raw))
	assertEqual(t, "path", line["path"], "/api/v1/callbacks/{token}")
	query, _ := line["query"].(string)
	if !strings.Contains(query, "x=1") || !strings.Contains(query, "redacted") {
		t.Fatalf("query = %q", query)
	}
	reqBody, _ := line["requestBody"].(string)
	if !strings.Contains(reqBody, "keep me") || !strings.Contains(reqBody, redactedValue) {
		t.Fatalf("requestBody = %q", reqBody)
	}
	respBody, _ := line["responseBody"].(string)
	if !strings.Contains(respBody, "expires_at") || !strings.Contains(respBody, redactedValue) {
		t.Fatalf("responseBody = %q", respBody)
	}
}

func TestRedactInjectBodyHidesCallbackToken(t *testing.T) {
	const marker = "aaaaaaaaaa"
	cases := []struct {
		name string
		body string
	}{
		{
			name: "inject",
			body: `{"taskId":"t1","taskCode":"review_v1","callbackToken":"` + marker + `","data":{}}`,
		},
		{
			name: "nested",
			body: `{"command":"submit","payload":{"callbackToken":"` + marker + `","note":"keep"}}`,
		},
		{
			name: "truncated",
			body: `{"callbackToken":"` + marker,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(redactLogged("application/json", []byte(tc.body)))
			if strings.Contains(got, marker) {
				t.Fatalf("callbackToken still present: %s", got)
			}
			if !strings.Contains(got, redactedValue) {
				t.Fatalf("body = %s", got)
			}
		})
	}
}

func TestRedactTruncatedJSONRemovesSignedURL(t *testing.T) {
	const signatureQuery = "X-Amz-Signature=truncated-marker"
	body := []byte(`{"upload_url":"https://s3.example/k?` + signatureQuery)
	got := string(redactLogged("application/json", body))
	if strings.Contains(got, signatureQuery) || strings.Contains(got, "s3.example") {
		t.Fatalf("truncated body still has the url: %s", got)
	}
	if !strings.Contains(got, redactedValue) {
		t.Fatalf("truncated body = %s", got)
	}
}

func TestMiddlewareLogsSwitchingProtocols(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws", nil))

	line := decodeLine(t, buf.Bytes())
	assertEqual(t, "status", line["status"], float64(http.StatusSwitchingProtocols))
}

type countReader struct {
	r io.Reader
	n int
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// finalStatus records the last non-1xx status and still accepts a body.
type finalStatus struct {
	http.ResponseWriter
	code int
}

func (f *finalStatus) WriteHeader(code int) {
	if code >= 100 && code <= 199 {
		return
	}
	f.code = code
	f.ResponseWriter.WriteHeader(code)
}

type plainReader struct{ io.Reader }

type sendFile struct {
	http.ResponseWriter
	buf  bytes.Buffer
	used bool
}

func (s *sendFile) ReadFrom(r io.Reader) (int64, error) {
	s.used = true
	return io.Copy(&s.buf, r)
}

func TestMiddlewareCarriesTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logging.NewHandler(slog.NewJSONHandler(&buf, nil)))

	h := trace.TraceMiddleware(Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/abc", nil)
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
