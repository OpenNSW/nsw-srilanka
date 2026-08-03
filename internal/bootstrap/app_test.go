package bootstrap

import (
	"net/http"
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
