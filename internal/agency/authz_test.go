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
func inputThrough(t *testing.T, gate *RoleGate, p *authn.Principal) (taskauthz.Input, bool) {
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

// ownedBy returns what gate reports a user principal owns of caseID.
func ownedBy(t *testing.T, gate *RoleGate, caseID string) map[string]bool {
	t.Helper()
	in, ok := inputThrough(t, gate, &authn.Principal{Kind: authn.KindUser, Roles: []string{"Officer"}})
	if !ok || in.Kind != taskauthz.KindUser || in.OwnedRoles == nil {
		t.Fatalf("input = %+v, ok = %v", in, ok)
	}
	owned, err := in.OwnedRoles(context.Background(), caseID)
	if err != nil {
		t.Fatal(err)
	}
	return owned
}

func TestRoleGateOwnership(t *testing.T) {
	repo := fakeRepo{injected: map[string]bool{"task-1": true}}
	gate := NewRoleGate(repo, []string{"officer", "reviewer"})

	t.Run("every catalog role owns injected workflows only", func(t *testing.T) {
		for root, want := range map[string]bool{"task-1": true, "consignment-1": false} {
			owned := ownedBy(t, gate, root)
			for _, role := range []string{"officer", "reviewer"} {
				if owned[role] != want {
					t.Errorf("%s: %s owned = %v, want %v", root, role, owned[role], want)
				}
			}
			// No trader/CHA ownership exists in agency mode.
			if owned["trader"] || owned["cha"] {
				t.Errorf("%s: owned = %v, want catalog roles only", root, owned)
			}
		}
	})

	t.Run("no case ID owns nothing", func(t *testing.T) {
		if owned := ownedBy(t, gate, ""); len(owned) != 0 {
			t.Fatalf("owned = %v, want none", owned)
		}
	})

	t.Run("a catalog without officer owns its own roles", func(t *testing.T) {
		owned := ownedBy(t, NewRoleGate(repo, []string{"reviewer"}), "task-1")
		if _, ok := owned["officer"]; ok || !owned["reviewer"] {
			t.Fatalf("owned = %v, want only reviewer", owned)
		}
	})
}

func TestRoleGate(t *testing.T) {
	gate := NewRoleGate(fakeRepo{injected: map[string]bool{"task-1": true}}, []string{"officer", "reviewer"})

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

// A user may act as a catalog role only when they hold its token role: ownership of an
// injected workflow is the same in every role, so the token decides.
func TestRoleGateEligibility(t *testing.T) {
	gate := NewRoleGate(fakeRepo{injected: map[string]bool{"task-1": true}}, []string{"officer", "reviewer"})
	cat := taskauthz.Catalog{Roles: map[string]string{"officer": "Officer", "reviewer": "Reviewer"}}

	for name, tc := range map[string]struct {
		tokenRoles           []string
		wantOfficer, wantRev bool
	}{
		"officer only":  {[]string{"Officer"}, true, false},
		"reviewer only": {[]string{"Reviewer"}, false, true},
		"both":          {[]string{"Officer", "Reviewer"}, true, true},
		"neither":       {[]string{"Trader"}, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			in, ok := inputThrough(t, gate, &authn.Principal{Kind: authn.KindUser, Roles: tc.tokenRoles})
			if !ok {
				t.Fatal("expected input")
			}
			e, err := cat.Eligible(context.Background(), in, "task-1", cat.RoleNames())
			if err != nil {
				t.Fatal(err)
			}
			if got := e.Allows("officer"); got != tc.wantOfficer {
				t.Errorf("officer allowed = %v, want %v", got, tc.wantOfficer)
			}
			if got := e.Allows("reviewer"); got != tc.wantRev {
				t.Errorf("reviewer allowed = %v, want %v", got, tc.wantRev)
			}
		})
	}
}

// A user carries their internal user id, the holder a claim is recorded under. A
// client has none.
func TestRoleGate_UserID(t *testing.T) {
	gate := NewRoleGate(fakeRepo{}, []string{"officer"})
	user, ok := inputThrough(t, gate, &authn.Principal{Kind: authn.KindUser, UserID: "user-1", Roles: []string{"Officer"}})
	if !ok || user.UserID != "user-1" {
		t.Errorf("user input = %+v, ok = %v; want UserID user-1", user, ok)
	}
	client, ok := inputThrough(t, gate, &authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA"})
	if !ok || client.UserID != "" {
		t.Errorf("client input = %+v, ok = %v; want no user id", client, ok)
	}
}
