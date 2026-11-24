package bootstrap

import (
	"net/http"
	"strings"
	"testing"
	"time"

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
