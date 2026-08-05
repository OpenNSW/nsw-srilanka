package authzgate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenNSW/core/authn"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	taskauthz "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
)

type fakeOwnership struct {
	trader, cha string
	err         error
	calls       int
}

func (f *fakeOwnership) GetOwnership(context.Context, string) (string, string, error) {
	f.calls++
	return f.trader, f.cha, f.err
}

type fakeCompany struct {
	id    string
	err   error
	calls int
}

func (f *fakeCompany) CompanyIDByOUHandle(context.Context, string) (string, error) {
	f.calls++
	return f.id, f.err
}

// validCatalogRoles satisfies validateCatalogRoles; tests unrelated to that
// check use it so NewMiddleware always succeeds.
var validCatalogRoles = map[string]string{roleTrader: "Trader", roleCHA: "CHA"}

// mustNewMiddleware builds a Middleware with validCatalogRoles, failing the
// test immediately if construction errors.
func mustNewMiddleware(t *testing.T, ownership OwnershipResolver, company CompanyResolver) *Middleware {
	t.Helper()
	mw, err := NewMiddleware(ownership, company, validCatalogRoles)
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}
	return mw
}

// attachedInput drives the middleware with an auth context and returns the Input
// the downstream handler observes.
func attachedInput(mw *Middleware, ac *authn.AuthContext) (taskauthz.Input, bool) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1", nil)
	if ac != nil {
		r = r.WithContext(context.WithValue(r.Context(), authn.AuthContextKey, ac))
	}
	var got taskauthz.Input
	var present bool
	next := http.HandlerFunc(func(_ http.ResponseWriter, rr *http.Request) {
		got, present = taskauthz.InputFromContext(rr.Context())
	})
	mw.Handler(next).ServeHTTP(httptest.NewRecorder(), r)
	return got, present
}

func TestMiddleware_Client(t *testing.T) {
	own := &fakeOwnership{}
	comp := &fakeCompany{}
	in, ok := attachedInput(mustNewMiddleware(t, own, comp), &authn.AuthContext{Client: &authn.ClientContext{ClientID: "FCAU_TO_NSW"}})

	if !ok || in.Kind != taskauthz.KindClient || in.ClientID != "FCAU_TO_NSW" {
		t.Fatalf("got %+v ok=%v", in, ok)
	}
	if in.OwnedRoles != nil {
		t.Error("client Input must not carry an ownership resolver")
	}
	if own.calls != 0 || comp.calls != 0 {
		t.Errorf("client must not trigger lookups: ownership=%d company=%d", own.calls, comp.calls)
	}
}

func TestMiddleware_Unauthenticated(t *testing.T) {
	if _, ok := attachedInput(mustNewMiddleware(t, &fakeOwnership{}, &fakeCompany{}), nil); ok {
		t.Fatal("want no Input for unauthenticated request")
	}
}

func TestMiddleware_UserResolverIsLazy(t *testing.T) {
	own := &fakeOwnership{trader: "adam-pvt-ltd", cha: "edward-pvt-ltd"}
	comp := &fakeCompany{id: "adam-pvt-ltd"}
	in, ok := attachedInput(mustNewMiddleware(t, own, comp), userCtx("adam-pvt-ltd", "Trader"))

	if !ok || in.Kind != taskauthz.KindUser || in.OwnedRoles == nil {
		t.Fatalf("got %+v ok=%v", in, ok)
	}
	// Attaching the Input must not have touched the DB.
	if own.calls != 0 || comp.calls != 0 {
		t.Fatalf("resolver ran eagerly: ownership=%d company=%d", own.calls, comp.calls)
	}

	owned, err := in.OwnedRoles(context.Background(), "c1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if own.calls != 1 || comp.calls != 1 {
		t.Errorf("expected one lookup each, got ownership=%d company=%d", own.calls, comp.calls)
	}
	if !owned["trader"] || owned["cha"] {
		t.Errorf("owned = %v, want trader only", owned)
	}
}

func TestMiddleware_UserResolverCases(t *testing.T) {
	const traderCo, chaCo = "adam-pvt-ltd", "edward-pvt-ltd"

	tests := []struct {
		name      string
		ownership *fakeOwnership
		company   *fakeCompany
		wantOwned map[string]bool
		wantErr   bool
	}{
		{"owns as cha", &fakeOwnership{trader: traderCo, cha: chaCo}, &fakeCompany{id: chaCo}, map[string]bool{"trader": false, "cha": true}, false},
		{"no company profile", &fakeOwnership{trader: traderCo, cha: chaCo}, &fakeCompany{id: ""}, map[string]bool{}, false},
		{"ownership error", &fakeOwnership{err: context.DeadlineExceeded}, &fakeCompany{id: traderCo}, nil, true},
		{"company error", &fakeOwnership{trader: traderCo}, &fakeCompany{err: context.DeadlineExceeded}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in, _ := attachedInput(mustNewMiddleware(t, tc.ownership, tc.company), userCtx("adam-pvt-ltd", "CHA"))
			owned, err := in.OwnedRoles(context.Background(), "c1")
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if len(owned) != len(tc.wantOwned) {
				t.Fatalf("owned = %v, want %v", owned, tc.wantOwned)
			}
			for k, v := range tc.wantOwned {
				if owned[k] != v {
					t.Errorf("owned[%q] = %v, want %v", k, owned[k], v)
				}
			}
		})
	}
}

func TestMiddleware_UserResolverShortCircuits(t *testing.T) {
	t.Run("empty ou handle skips all lookups", func(t *testing.T) {
		own := &fakeOwnership{trader: "adam-pvt-ltd"}
		comp := &fakeCompany{id: "adam-pvt-ltd"}
		in, _ := attachedInput(mustNewMiddleware(t, own, comp), userCtx("", "Trader"))
		owned, err := in.OwnedRoles(context.Background(), "c1")
		if err != nil || len(owned) != 0 {
			t.Fatalf("owned=%v err=%v, want empty/nil", owned, err)
		}
		if own.calls != 0 || comp.calls != 0 {
			t.Errorf("expected no lookups, got ownership=%d company=%d", own.calls, comp.calls)
		}
	})

	t.Run("empty root workflow id skips all lookups", func(t *testing.T) {
		own := &fakeOwnership{trader: "adam-pvt-ltd"}
		comp := &fakeCompany{id: "adam-pvt-ltd"}
		in, _ := attachedInput(mustNewMiddleware(t, own, comp), userCtx("adam-pvt-ltd", "Trader"))
		owned, err := in.OwnedRoles(context.Background(), "")
		if err != nil || len(owned) != 0 {
			t.Fatalf("owned=%v err=%v, want empty/nil", owned, err)
		}
		if own.calls != 0 || comp.calls != 0 {
			t.Errorf("expected no lookups, got ownership=%d company=%d", own.calls, comp.calls)
		}
	})

	t.Run("no company profile skips ownership lookup", func(t *testing.T) {
		own := &fakeOwnership{trader: "adam-pvt-ltd"}
		comp := &fakeCompany{id: ""} // no profile
		in, _ := attachedInput(mustNewMiddleware(t, own, comp), userCtx("ghost", "Trader"))
		owned, err := in.OwnedRoles(context.Background(), "c1")
		if err != nil || len(owned) != 0 {
			t.Fatalf("owned=%v err=%v, want empty/nil", owned, err)
		}
		if own.calls != 0 {
			t.Errorf("ownership lookup should be skipped when the caller has no company, got %d", own.calls)
		}
	})
}

func TestValidateCatalogRoles(t *testing.T) {
	tests := []struct {
		name    string
		roles   map[string]string
		wantErr string // substring expected in the error; "" means no error
	}{
		{name: "both present", roles: map[string]string{"trader": "Trader", "cha": "CHA"}},
		{name: "extra roles ignored", roles: map[string]string{"trader": "Trader", "cha": "CHA", "fcau": "FCAU_TO_NSW"}},
		{name: "missing cha", roles: map[string]string{"trader": "Trader"}, wantErr: "cha"},
		{name: "missing trader", roles: map[string]string{"cha": "CHA"}, wantErr: "trader"},
		{name: "missing both", roles: map[string]string{}, wantErr: "trader, cha"},
		{name: "nil map", roles: nil, wantErr: "trader, cha"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCatalogRoles(tc.roles)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCatalogRoles(%v) = %v, want nil", tc.roles, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateCatalogRoles(%v) = %v, want error containing %q", tc.roles, err, tc.wantErr)
			}
		})
	}
}

// TestOwnedRoleKeysMatchCatalog pins the logical names ownedRolesFor hardcodes to
// the shipped catalog by running the exact check NewMiddleware runs at
// construction. The authz extension looks up the same names in the catalog's
// roles, so a rename there would leave the role match succeeding while
// ownership silently resolved false for every candidate — a blanket 403 with no
// obvious cause. Fail here instead.
func TestOwnedRoleKeysMatchCatalog(t *testing.T) {
	c, err := catalog.Load(filepath.Join("..", "..", "..", "configs", "catalog.example.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if err := validateCatalogRoles(c.Roles); err != nil {
		t.Error(err)
	}
}

func userCtx(ouHandle string, roles ...string) *authn.AuthContext {
	return &authn.AuthContext{User: &authn.UserContext{OUHandle: ouHandle, Roles: roles}}
}
