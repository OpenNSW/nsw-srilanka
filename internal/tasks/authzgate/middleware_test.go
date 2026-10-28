package authzgate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/caseparty"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// fakeOwnership stands in for *caseparty.Repository: holders maps a company id to the
// roles it holds on every case.
type fakeOwnership struct {
	holders map[string][]string
	err     error
	calls   int
	parties []caseparty.Ref
}

func (f *fakeOwnership) RolesOf(_ context.Context, _ string, parties ...caseparty.Ref) (map[string]bool, error) {
	f.calls++
	f.parties = parties
	if f.err != nil {
		return nil, f.err
	}
	roles := map[string]bool{}
	for _, p := range parties {
		for _, role := range f.holders[p.ID] {
			roles[role] = true
		}
	}
	return roles, nil
}

// owns builds a fakeOwnership where trader holds "trader" and cha holds "cha".
func owns(trader, cha string) *fakeOwnership {
	holders := map[string][]string{}
	if trader != "" {
		holders[trader] = append(holders[trader], "trader")
	}
	if cha != "" {
		holders[cha] = append(holders[cha], "cha")
	}
	return &fakeOwnership{holders: holders}
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

// mustNewMiddleware builds a Middleware, failing the test immediately if
// construction errors.
func mustNewMiddleware(t *testing.T, ownership CaseRoles, company CompanyResolver) *Middleware {
	t.Helper()
	mw, err := NewMiddleware(ownership, company)
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}
	return mw
}

// attachedInput drives the middleware with a Principal and returns the Input
// the downstream handler observes.
func attachedInput(mw *Middleware, p *authn.Principal) (taskauthz.Input, bool) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1", nil)
	if p != nil {
		r = r.WithContext(authn.ContextWithPrincipal(r.Context(), p))
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
	in, ok := attachedInput(mustNewMiddleware(t, own, comp), &authn.Principal{Kind: authn.KindClient, ClientID: "FCAU_TO_NSW"})

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
	own := owns("adam-pvt-ltd", "edward-pvt-ltd")
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
		{"owns as cha", owns(traderCo, chaCo), &fakeCompany{id: chaCo}, map[string]bool{"cha": true}, false},
		{"owns as both", owns(traderCo, traderCo), &fakeCompany{id: traderCo}, map[string]bool{"trader": true, "cha": true}, false},
		{"owns nothing", owns(traderCo, chaCo), &fakeCompany{id: "other-co"}, map[string]bool{}, false},
		{"no company profile", owns(traderCo, chaCo), &fakeCompany{id: ""}, map[string]bool{}, false},
		{"ownership error", &fakeOwnership{err: context.DeadlineExceeded}, &fakeCompany{id: traderCo}, nil, true},
		{"company error", owns(traderCo, ""), &fakeCompany{err: context.DeadlineExceeded}, nil, true},
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
		own := owns("adam-pvt-ltd", "")
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

	t.Run("empty case id skips all lookups", func(t *testing.T) {
		own := owns("adam-pvt-ltd", "")
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
		own := owns("adam-pvt-ltd", "")
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

// Both dependencies are required: a nil one would panic on the first request.
func TestNewMiddleware_RequiresDependencies(t *testing.T) {
	if _, err := NewMiddleware(nil, &fakeCompany{}); err == nil {
		t.Error("NewMiddleware(nil roles) = nil error, want one")
	}
	if _, err := NewMiddleware(&fakeOwnership{}, nil); err == nil {
		t.Error("NewMiddleware(nil company) = nil error, want one")
	}
}

// The resolver asks for the roles of the caller's company, the party a trader or CHA
// owns a consignment as.
func TestMiddleware_AsksForCallerCompany(t *testing.T) {
	own := owns("adam-pvt-ltd", "")
	in, _ := attachedInput(mustNewMiddleware(t, own, &fakeCompany{id: "adam-pvt-ltd"}), userCtx("adam-pvt-ltd", "Trader"))
	if _, err := in.OwnedRoles(context.Background(), "c1"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []caseparty.Ref{caseparty.Company("adam-pvt-ltd")}
	if len(own.parties) != 1 || own.parties[0] != want[0] {
		t.Errorf("parties = %v, want %v", own.parties, want)
	}
}

func userCtx(ouHandle string, roles ...string) *authn.Principal {
	return &authn.Principal{Kind: authn.KindUser, OUHandle: ouHandle, Roles: roles}
}

// A user carries their internal user id, the holder a claim is recorded under. A
// client has none.
func TestMiddleware_UserCarriesUserID(t *testing.T) {
	mw := mustNewMiddleware(t, &fakeOwnership{}, &fakeCompany{})
	p := userCtx("adam-pvt-ltd", "Trader")
	p.UserID = "user-1"
	in, ok := attachedInput(mw, p)
	if !ok || in.UserID != "user-1" {
		t.Errorf("user input = %+v, ok = %v; want UserID user-1", in, ok)
	}
	client, _ := attachedInput(mw, &authn.Principal{Kind: authn.KindClient, ClientID: "FCAU_TO_NSW"})
	if client.UserID != "" {
		t.Errorf("client input = %+v, want no user id", client)
	}
}
