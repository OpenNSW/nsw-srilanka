package readauthz

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

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

// A catalog with no roles makes nobody eligible, so every read must be denied
// rather than waved through. Unreachable in the real wiring — the authz gate
// fronting this route already refuses to construct without both owner roles —
// but the fail-closed direction is worth pinning.
func TestAuthorize_EmptyCatalogDenies(t *testing.T) {
	calls := 0
	in := userInput([]string{"Trader"}, map[string]bool{"trader": true}, &calls)

	err := Authorize(context.Background(), taskauthz.Catalog{Clients: testCatalog.Clients}, in, "consignment-1")
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("got %v, want ErrDenied", err)
	}
}

// Eligibility is held-role AND owned-slot. Every case below fixes one and varies
// the other, so a regression dropping either conjunct fails here.
func TestAuthorize_RoleTiedOwnership(t *testing.T) {
	tests := []struct {
		name         string
		heldRoles    []string
		owned        map[string]bool
		wantDenied   bool
		wantResolves int
	}{
		{
			name:         "trader at the trader company is eligible as trader only",
			heldRoles:    []string{"Trader"},
			owned:        map[string]bool{"trader": true, "cha": false},
			wantResolves: 1,
		},
		{
			name:         "cha at the cha company is eligible as cha only",
			heldRoles:    []string{"CHA"},
			owned:        map[string]bool{"trader": false, "cha": true},
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
			wantResolves: 1,
		},
		{
			name:         "both roles held and both slots owned is eligible for both",
			heldRoles:    []string{"Trader", "CHA"},
			owned:        map[string]bool{"trader": true, "cha": true},
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

			err := Authorize(context.Background(), testCatalog, in, "consignment-1")
			if tt.wantDenied {
				if !errors.Is(err, ErrDenied) {
					t.Fatalf("got %v, want ErrDenied", err)
				}
			} else if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			if calls != tt.wantResolves {
				t.Errorf("ownership resolved %d times, want %d", calls, tt.wantResolves)
			}
		})
	}
}

// Non-user principals own nothing, and saying so must not cost a lookup.
func TestAuthorize_NonUserPrincipalsDenyWithoutLookup(t *testing.T) {
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
			err := Authorize(context.Background(), testCatalog, in, "consignment-1")
			if !errors.Is(err, ErrDenied) {
				t.Fatalf("got %v, want ErrDenied", err)
			}
		})
	}
	if calls != 0 {
		t.Errorf("ownership resolved %d times, want 0", calls)
	}
}

func TestAuthorize_OwnershipErrorIsNotADenial(t *testing.T) {
	sentinel := errors.New("boom")
	in := taskauthz.Input{
		Kind:  taskauthz.KindUser,
		Roles: []string{"Trader"},
		OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return nil, sentinel
		},
	}

	err := Authorize(context.Background(), testCatalog, in, "consignment-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the resolver's error wrapped", err)
	}
	if errors.Is(err, ErrDenied) {
		t.Error("a failed ownership lookup must not be reported as a denial")
	}
}
