package readauthz

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// noPolicy declares no read.roles, so no precedence applies.
const noPolicy = `{"id":"x:render","sections":{}}`

var testCatalog = taskauthz.Catalog{
	Roles:   map[string]string{"trader": "Trader", "cha": "CHA"},
	Clients: map[string]string{"fcau": "FCAU_TO_NSW"},
}

// ownedBy returns a resolver reporting the given ownership, counting calls so
// tests can assert the deny paths never touch the database.
func ownedBy(owned map[string]bool, calls *int) taskauthz.OwnedRolesFunc {
	return func(context.Context, string) (map[string]bool, error) {
		*calls++
		return owned, nil
	}
}

func userInput(heldRoles []string, owned map[string]bool, calls *int) taskauthz.Input {
	return taskauthz.Input{
		Kind:       taskauthz.KindUser,
		Roles:      heldRoles,
		OwnedRoles: ownedBy(owned, calls),
	}
}

func assertClaims(t *testing.T, got, want map[string]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want exactly the keys %v", got, want)
	}
	for key, w := range want {
		if got[key] != w {
			t.Errorf("claims[%q] = %v, want %v", key, got[key], w)
		}
	}
}

// A catalog with no roles makes nobody eligible, so every read must be denied
// rather than waved through. Unreachable in the real wiring — the authz gate
// fronting this route already refuses to construct without both owner roles —
// but the fail-closed direction is worth pinning.
func TestResolve_EmptyCatalogDenies(t *testing.T) {
	calls := 0
	in := userInput([]string{"Trader"}, map[string]bool{"trader": true}, &calls)

	_, err := Resolve(context.Background(), taskauthz.Catalog{Clients: testCatalog.Clients}, in, json.RawMessage(noPolicy), "consignment-1")
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("got %v, want ErrDenied", err)
	}
}

// Eligibility is held-role AND owned-slot. Every case below fixes one and varies
// the other, so a regression dropping either conjunct fails here.
func TestResolve_RoleTiedOwnership(t *testing.T) {
	tests := []struct {
		name         string
		heldRoles    []string
		owned        map[string]bool
		wantClaims   map[string]bool
		wantDenied   bool
		wantResolves int
	}{
		{
			name:         "trader at the trader company is eligible as trader only",
			heldRoles:    []string{"Trader"},
			owned:        map[string]bool{"trader": true, "cha": false},
			wantClaims:   map[string]bool{"role:trader": true, "role:cha": false},
			wantResolves: 1,
		},
		{
			name:         "cha at the cha company is eligible as cha only",
			heldRoles:    []string{"CHA"},
			owned:        map[string]bool{"trader": false, "cha": true},
			wantClaims:   map[string]bool{"role:trader": false, "role:cha": true},
			wantResolves: 1,
		},
		{
			// The company sits in the CHA slot but the caller holds no CHA role, so
			// they own nothing here. This is the cross-slot hole issue #272 describes.
			name:         "trader-only token at the cha company is eligible for nothing",
			heldRoles:    []string{"Trader"},
			owned:        map[string]bool{"trader": false, "cha": true},
			wantDenied:   true,
			wantResolves: 1,
		},
		{
			// A self-clearing company fills both slots; only the role it holds counts.
			name:         "company in both slots gets only the role it holds",
			heldRoles:    []string{"CHA"},
			owned:        map[string]bool{"trader": true, "cha": true},
			wantClaims:   map[string]bool{"role:trader": false, "role:cha": true},
			wantResolves: 1,
		},
		{
			name:         "both roles held and both slots owned is eligible for both",
			heldRoles:    []string{"Trader", "CHA"},
			owned:        map[string]bool{"trader": true, "cha": true},
			wantClaims:   map[string]bool{"role:trader": true, "role:cha": true},
			wantResolves: 1,
		},
		{
			name:         "cha token before a cha is assigned is eligible for nothing",
			heldRoles:    []string{"CHA"},
			owned:        map[string]bool{"trader": false, "cha": false},
			wantDenied:   true,
			wantResolves: 1,
		},
		{
			// No catalog role held, so nothing could be owned in any role: skip the
			// lookup rather than paying for a foregone conclusion.
			name:         "caller holding no catalog role never resolves ownership",
			heldRoles:    []string{"SomeOtherRole"},
			owned:        map[string]bool{"trader": true, "cha": true},
			wantDenied:   true,
			wantResolves: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			in := userInput(tt.heldRoles, tt.owned, &calls)

			got, err := Resolve(context.Background(), testCatalog, in, json.RawMessage(noPolicy), "consignment-1")
			if tt.wantDenied {
				if !errors.Is(err, ErrDenied) {
					t.Fatalf("got (%v, %v), want ErrDenied", got, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Resolve: %v", err)
				}
				assertClaims(t, got, tt.wantClaims)
			}
			if calls != tt.wantResolves {
				t.Errorf("ownership resolved %d times, want %d", calls, tt.wantResolves)
			}
		})
	}
}

// Non-user principals own nothing, and saying so must not cost a lookup.
func TestResolve_NonUserPrincipalsDenyWithoutLookup(t *testing.T) {
	calls := 0

	for name, in := range map[string]taskauthz.Input{
		"client principal": {
			Kind:       taskauthz.KindClient,
			ClientID:   "FCAU_TO_NSW",
			OwnedRoles: ownedBy(map[string]bool{"trader": true, "cha": true}, &calls),
		},
		"no ownership func": {Kind: taskauthz.KindUser, Roles: []string{"Trader"}},
		"unknown kind":      {},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(context.Background(), testCatalog, in, json.RawMessage(noPolicy), "consignment-1")
			if !errors.Is(err, ErrDenied) {
				t.Fatalf("got %v, want ErrDenied", err)
			}
		})
	}
	if calls != 0 {
		t.Errorf("ownership resolved %d times, want 0", calls)
	}
}

func TestResolve_OwnershipErrorIsNotADenial(t *testing.T) {
	sentinel := errors.New("boom")
	in := taskauthz.Input{
		Kind:  taskauthz.KindUser,
		Roles: []string{"Trader"},
		OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return nil, sentinel
		},
	}

	_, err := Resolve(context.Background(), testCatalog, in, json.RawMessage(noPolicy), "consignment-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the resolver's error wrapped", err)
	}
	if errors.Is(err, ErrDenied) {
		t.Error("a failed ownership lookup must not be reported as a denial")
	}
}

// The dual-role case: a user holding Trader and CHA at a company filling both
// slots is eligible for both, and would otherwise render two contradictory
// sections at once. read.roles order picks exactly one.
func TestResolve_ReadRolesOrderIsPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		heldRoles  []string
		owned      map[string]bool
		wantClaims map[string]bool
	}{
		{
			name:       "dual-role user acts as trader when trader is listed first",
			config:     `{"read":{"roles":["trader","cha"]}}`,
			heldRoles:  []string{"Trader", "CHA"},
			owned:      map[string]bool{"trader": true, "cha": true},
			wantClaims: map[string]bool{"role:trader": true, "role:cha": false},
		},
		{
			name:       "the same user acts as cha when cha is listed first",
			config:     `{"read":{"roles":["cha","trader"]}}`,
			heldRoles:  []string{"Trader", "CHA"},
			owned:      map[string]bool{"trader": true, "cha": true},
			wantClaims: map[string]bool{"role:trader": false, "role:cha": true},
		},
		{
			// Precedence only chooses among roles the caller is actually eligible
			// for; it never grants the higher-precedence one.
			name:       "precedence skips a listed role the caller is not eligible for",
			config:     `{"read":{"roles":["trader","cha"]}}`,
			heldRoles:  []string{"CHA"},
			owned:      map[string]bool{"trader": true, "cha": true},
			wantClaims: map[string]bool{"role:trader": false, "role:cha": true},
		},
		{
			name:       "a single-role user is unaffected by the order",
			config:     `{"read":{"roles":["cha","trader"]}}`,
			heldRoles:  []string{"Trader"},
			owned:      map[string]bool{"trader": true, "cha": false},
			wantClaims: map[string]bool{"role:trader": true, "role:cha": false},
		},
		{
			// An undefined name cannot be owned by anyone, so it drops out of the
			// order without shadowing the real entry behind it.
			name:       "unknown names drop out of the precedence order",
			config:     `{"read":{"roles":["chaa","trader"]}}`,
			heldRoles:  []string{"Trader", "CHA"},
			owned:      map[string]bool{"trader": true, "cha": true},
			wantClaims: map[string]bool{"role:trader": true, "role:cha": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			in := userInput(tt.heldRoles, tt.owned, &calls)

			got, err := Resolve(context.Background(), testCatalog, in, json.RawMessage(tt.config), "consignment-1")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			assertClaims(t, got, tt.wantClaims)

			trues := 0
			for _, v := range got {
				if v {
					trues++
				}
			}
			if trues != 1 {
				t.Errorf("got %d true claims %v, want exactly 1 when read.roles declares precedence", trues, got)
			}
		})
	}
}

func TestResolve_ReadRolesGatesAccess(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		heldRoles  []string
		owned      map[string]bool
		wantDenied bool
	}{
		{
			name:      "listed role is eligible",
			config:    `{"read":{"roles":["trader","cha"]}}`,
			heldRoles: []string{"Trader"},
			owned:     map[string]bool{"trader": true},
		},
		{
			name:       "eligible role is not listed",
			config:     `{"read":{"roles":["cha"]}}`,
			heldRoles:  []string{"Trader"},
			owned:      map[string]bool{"trader": true},
			wantDenied: true,
		},
		{
			name:      "empty roles list falls back to any eligible role",
			config:    `{"read":{"roles":[]}}`,
			heldRoles: []string{"Trader"},
			owned:     map[string]bool{"trader": true},
		},
		{
			name:      "absent render config admits any eligible role",
			config:    "",
			heldRoles: []string{"Trader"},
			owned:     map[string]bool{"trader": true},
		},
		{
			name:       "a policy of only unknown names admits nobody",
			config:     `{"read":{"roles":["chaa"]}}`,
			heldRoles:  []string{"Trader", "CHA"},
			owned:      map[string]bool{"trader": true, "cha": true},
			wantDenied: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			in := userInput(tt.heldRoles, tt.owned, &calls)

			_, err := Resolve(context.Background(), testCatalog, in, json.RawMessage(tt.config), "consignment-1")
			if tt.wantDenied {
				if !errors.Is(err, ErrDenied) {
					t.Fatalf("got %v, want ErrDenied", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
		})
	}
}

// uiprojector treats a claim its blueprint references but the caller never
// populated as a caller bug and fails the whole render. Every catalog role must
// therefore always be present, denials included.
func TestResolve_PopulatesEveryCatalogRole(t *testing.T) {
	calls := 0

	for name, cfg := range map[string]string{
		"no read policy":  noPolicy,
		"with precedence": `{"read":{"roles":["cha","trader"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			in := userInput([]string{"CHA"}, map[string]bool{"cha": true}, &calls)
			got, err := Resolve(context.Background(), testCatalog, in, json.RawMessage(cfg), "consignment-1")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			for role := range testCatalog.Roles {
				if _, ok := got[ClaimKey(role)]; !ok {
					t.Errorf("claim %q missing; every catalog role must be populated", ClaimKey(role))
				}
			}
		})
	}
}

// A malformed render config is a real failure, not a denial: a 500 says "fix the
// config", a 404 would quietly hide the task instead.
func TestResolve_MalformedConfigIsNotADenial(t *testing.T) {
	calls := 0
	in := userInput([]string{"Trader"}, map[string]bool{"trader": true}, &calls)

	_, err := Resolve(context.Background(), testCatalog, in, json.RawMessage(`{"read":`), "consignment-1")
	if err == nil {
		t.Fatal("want an error for a malformed render config, got none")
	}
	if errors.Is(err, ErrDenied) {
		t.Error("a malformed config must not be reported as a denial")
	}
}

// TestClaimKeysMatchCatalog pins the claim keys this package produces to the
// shipped catalog. Render configs name claims as literal strings, so renaming a
// catalog role would leave every requireClaim in the artifacts referring to a
// claim nothing produces — which uiprojector reports as a caller bug, failing
// every read of those tasks. Fail here instead.
func TestClaimKeysMatchCatalog(t *testing.T) {
	c, err := catalog.Load(filepath.Join("..", "..", "..", "configs", "catalog.example.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	cat := taskauthz.Catalog{Roles: c.Roles, Clients: c.Clients}

	calls := 0
	in := userInput([]string{"Trader", "CHA"}, map[string]bool{"trader": true, "cha": true}, &calls)
	claims, err := Resolve(context.Background(), cat, in, json.RawMessage(noPolicy), "consignment-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, want := range []string{"role:trader", "role:cha"} {
		if _, ok := claims[want]; !ok {
			t.Errorf("claim %q is referenced by the shipped render configs but not produced; got %v", want, claims)
		}
	}
}
