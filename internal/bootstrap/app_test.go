package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenNSW/core/platform/authn"
	"github.com/stretchr/testify/assert"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

func TestNewHTTPServerUsesConfiguredTimeouts(t *testing.T) {
	server := newHTTPServer(config.ServerConfig{
		Port:              9090,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       7 * time.Second,
		WriteTimeout:      9 * time.Second,
		IdleTimeout:       11 * time.Second,
	}, http.NewServeMux())

	if got, want := server.Addr, ":9090"; got != want {
		t.Fatalf("Addr = %q, want %q", got, want)
	}
	if got, want := server.ReadHeaderTimeout, 2*time.Second; got != want {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", got, want)
	}
	if got, want := server.ReadTimeout, 7*time.Second; got != want {
		t.Fatalf("ReadTimeout = %v, want %v", got, want)
	}
	if got, want := server.WriteTimeout, 9*time.Second; got != want {
		t.Fatalf("WriteTimeout = %v, want %v", got, want)
	}
	if got, want := server.IdleTimeout, 11*time.Second; got != want {
		t.Fatalf("IdleTimeout = %v, want %v", got, want)
	}
}

func TestLogRequestLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int64
		wantWarn bool
	}{
		{"below the recommended limit", config.RecommendedMaxRequestBytes / 2, false},
		{"at the recommended limit", config.RecommendedMaxRequestBytes, false},
		{"above the recommended limit", 32 << 20, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogs(t)

			logRequestLimit(config.ServerConfig{MaxRequestBytes: tt.limit})

			if got := strings.Contains(logs.String(), "server.maxRequestBytes is above the recommended limit"); got != tt.wantWarn {
				t.Fatalf("warned = %v, want %v; logs: %s", got, tt.wantWarn, logs.String())
			}
		})
	}
}

// server.strictRequestBodies gives each lenient handler its WithStrictRequestBodies, and
// the file handler built with them refuses an upload request field it does not declare.
func TestRequestBodyOptions(t *testing.T) {
	assert.Equal(t, bodyOptions{}, requestBodyOptions(config.ServerConfig{}))

	strict := requestBodyOptions(config.ServerConfig{StrictRequestBodies: true})
	assert.Len(t, strict.files, 1)
	assert.Len(t, strict.tasks, 1)
	assert.Len(t, strict.start, 1)

	alice := &authn.Principal{Kind: authn.KindUser, IDPUserID: "alice"}
	const body = `{"filename":"invoice.pdf","mime_type":"application/pdf","size":12,"note":"x"}`
	for name, tt := range map[string]struct {
		opts       bodyOptions
		wantStatus int
	}{
		"unset":  {opts: bodyOptions{}, wantStatus: http.StatusOK},
		"strict": {opts: strict, wantStatus: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, h := newTestStorageHandler(t, nil, tt.opts.files...)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/storage", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.Upload(rec, req.WithContext(authn.ContextWithPrincipal(req.Context(), alice)))
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
