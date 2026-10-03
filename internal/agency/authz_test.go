package agency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

type fakeRepo struct {
	Repository
	injected map[string]bool
}

func (f fakeRepo) Exists(_ context.Context, taskID string) (bool, error) {
	return f.injected[taskID], nil
}

// inputThrough runs gate for a request authenticated as p (unauthenticated when nil)
// and returns the Input it attached.
func inputThrough(t *testing.T, gate *OfficerGate, p *authn.Principal) (taskauthz.Input, bool) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if p != nil {
		r = r.WithContext(authn.ContextWithPrincipal(r.Context(), p))
	}
	var in taskauthz.Input
	var ok bool
	gate.Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		in, ok = taskauthz.InputFromContext(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), r)
	return in, ok
}

func TestOfficerGate(t *testing.T) {
	gate := NewOfficerGate(fakeRepo{injected: map[string]bool{"task-1": true}})

	t.Run("officer owns injected workflows only", func(t *testing.T) {
		in, ok := inputThrough(t, gate, &authn.Principal{Kind: authn.KindUser, Roles: []string{"Officer"}})
		if !ok || in.Kind != taskauthz.KindUser || in.OwnedRoles == nil {
			t.Fatalf("input = %+v, ok = %v", in, ok)
		}
		for root, want := range map[string]bool{"task-1": true, "consignment-1": false} {
			owned, err := in.OwnedRoles(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			if owned[RoleOfficer] != want {
				t.Errorf("%s: officer owned = %v, want %v", root, owned[RoleOfficer], want)
			}
			// No trader/CHA ownership exists in agency mode.
			if owned["trader"] || owned["cha"] {
				t.Errorf("%s: owned = %v, want officer only", root, owned)
			}
		}
	})

	t.Run("client carries its id and no ownership", func(t *testing.T) {
		in, ok := inputThrough(t, gate, &authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA"})
		if !ok || in.Kind != taskauthz.KindClient || in.ClientID != "NSW_TO_CDA" || in.OwnedRoles != nil {
			t.Fatalf("input = %+v, ok = %v", in, ok)
		}
	})

	t.Run("unauthenticated gets no input", func(t *testing.T) {
		if _, ok := inputThrough(t, gate, nil); ok {
			t.Fatal("expected no input")
		}
	})
}
