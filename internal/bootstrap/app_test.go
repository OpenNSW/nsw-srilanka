package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

// The Helm chart's frontend.waitForBackend init container waits for the
// "version" field of /health to equal the backend image tag it deploys.
func TestHealthResponseReportsVersion(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, healthResponse{Status: "ok", Service: "tnsw-api", Version: "1.2.3"})

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got, want := body["version"], "1.2.3"; got != want {
		t.Fatalf(`body["version"] = %v, want %q`, got, want)
	}
}

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
