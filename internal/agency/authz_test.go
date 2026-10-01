package agency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

type fakeRepo struct {
	Repository
	injected map[string]bool
}

func (f fakeRepo) Exists(_ context.Context, taskID string) (bool, error) {
	return f.injected[taskID], nil
}

// stubGate stands in for authzgate: it attaches in, as Layer 1 would.
type stubGate struct{ in taskauthz.Input }

func (g *stubGate) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(taskauthz.WithInput(r.Context(), g.in)))
	})
}

// ownedThrough runs gate and returns what the Input it attached resolves for rootID.
func ownedThrough(t *testing.T, gate TaskGate, rootID string) map[string]bool {
	t.Helper()
	var owned map[string]bool
	gate.Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		in, ok := taskauthz.InputFromContext(r.Context())
		if !ok {
			t.Fatal("no input attached")
		}
		var err error
		if owned, err = in.OwnedRoles(r.Context(), rootID); err != nil {
			t.Fatalf("OwnedRoles: %v", err)
		}
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	return owned
}

func TestWrapTaskGate(t *testing.T) {
	base := func(context.Context, string) (map[string]bool, error) {
		return map[string]bool{"trader": true, "cha": false}, nil
	}
	gate := &stubGate{in: taskauthz.Input{Kind: taskauthz.KindUser, OwnedRoles: base}}
	repo := fakeRepo{injected: map[string]bool{"task-1": true}}
	roles := map[string]string{"trader": "Trader", "cha": "CHA", RoleOfficer: "Officer"}

	t.Run("catalog without officer leaves the gate unchanged", func(t *testing.T) {
		if got := WrapTaskGate(gate, repo, map[string]string{"trader": "Trader"}); got != TaskGate(gate) {
			t.Fatalf("expected the original gate, got %T", got)
		}
	})

	t.Run("injected workflow grants officer and keeps base ownership", func(t *testing.T) {
		owned := ownedThrough(t, WrapTaskGate(gate, repo, roles), "task-1")
		if !owned[RoleOfficer] || !owned["trader"] {
			t.Fatalf("owned = %v, want officer and trader", owned)
		}
	})

	t.Run("other workflows do not grant officer", func(t *testing.T) {
		if owned := ownedThrough(t, WrapTaskGate(gate, repo, roles), "consignment-1"); owned[RoleOfficer] {
			t.Fatalf("owned = %v, want no officer", owned)
		}
	})

	t.Run("user without a base resolver still gets officer", func(t *testing.T) {
		g := &stubGate{in: taskauthz.Input{Kind: taskauthz.KindUser}}
		if owned := ownedThrough(t, WrapTaskGate(g, repo, roles), "task-1"); !owned[RoleOfficer] {
			t.Fatalf("owned = %v, want officer", owned)
		}
	})
}
