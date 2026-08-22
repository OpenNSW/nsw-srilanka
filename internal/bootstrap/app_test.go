package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/internal/consignment"
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

// fakeOwnership stands in for *consignment.Service.
type fakeOwnership struct {
	trader, cha string
	err         error
}

func (f fakeOwnership) GetOwnership(context.Context, string) (string, string, error) {
	return f.trader, f.cha, f.err
}

// A consignment that no longer exists must deny the caller, not fail the request.
// authzgate calls GetOwnership while resolving a caller's owned roles; before this
// adapter, consignment.ErrConsignmentNotFound propagated as a non-sentinel error
// and the task-write handler mapped it to a 500. Reporting "no owner" instead
// leaves the caller owning neither side, which the policy layer denies.
func TestOwnershipResolverReportsMissingConsignmentAsNoOwner(t *testing.T) {
	r := ownershipResolver{svc: fakeOwnership{err: consignment.ErrConsignmentNotFound}}

	trader, cha, err := r.GetOwnership(context.Background(), "gone")
	if err != nil {
		t.Fatalf("missing consignment: got error %v, want nil so the caller is denied cleanly", err)
	}
	if trader != "" || cha != "" {
		t.Errorf("missing consignment: got (%q, %q), want both empty", trader, cha)
	}
}

// Any other failure is a real one and must not be flattened into a denial:
// silently reporting "no owner" would turn a database outage into a blanket 404.
func TestOwnershipResolverPropagatesRealFailures(t *testing.T) {
	boom := errors.New("connection refused")
	r := ownershipResolver{svc: fakeOwnership{err: boom}}

	if _, _, err := r.GetOwnership(context.Background(), "c-1"); !errors.Is(err, boom) {
		t.Errorf("got %v, want the underlying error propagated", err)
	}
}

// The pass-through case: an existing consignment reports its two owner companies
// unchanged.
func TestOwnershipResolverPassesOwnersThrough(t *testing.T) {
	r := ownershipResolver{svc: fakeOwnership{trader: "acme", cha: "clearco"}}

	trader, cha, err := r.GetOwnership(context.Background(), "c-1")
	if err != nil {
		t.Fatalf("GetOwnership: %v", err)
	}
	if trader != "acme" || cha != "clearco" {
		t.Errorf("got (%q, %q), want (\"acme\", \"clearco\")", trader, cha)
	}
}
