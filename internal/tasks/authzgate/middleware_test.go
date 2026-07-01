package authzgate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/core/authn"

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
	in, ok := attachedInput(NewMiddleware(own, comp), &authn.AuthContext{Client: &authn.ClientContext{ClientID: "FCAU_TO_NSW"}})

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
	if _, ok := attachedInput(NewMiddleware(&fakeOwnership{}, &fakeCompany{}), nil); ok {
		t.Fatal("want no Input for unauthenticated request")
	}
}

func TestMiddleware_UserResolverIsLazy(t *testing.T) {
	own := &fakeOwnership{trader: "adam-pvt-ltd", cha: "edward-pvt-ltd"}
	comp := &fakeCompany{id: "adam-pvt-ltd"}
	in, ok := attachedInput(NewMiddleware(own, comp), userCtx("adam-pvt-ltd", "Trader"))

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
			in, _ := attachedInput(NewMiddleware(tc.ownership, tc.company), userCtx("adam-pvt-ltd", "CHA"))
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
		in, _ := attachedInput(NewMiddleware(own, comp), userCtx("", "Trader"))
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
		in, _ := attachedInput(NewMiddleware(own, comp), userCtx("adam-pvt-ltd", "Trader"))
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
		in, _ := attachedInput(NewMiddleware(own, comp), userCtx("ghost", "Trader"))
		owned, err := in.OwnedRoles(context.Background(), "c1")
		if err != nil || len(owned) != 0 {
			t.Fatalf("owned=%v err=%v, want empty/nil", owned, err)
		}
		if own.calls != 0 {
			t.Errorf("ownership lookup should be skipped when the caller has no company, got %d", own.calls)
		}
	})
}

func userCtx(ouHandle string, roles ...string) *authn.AuthContext {
	return &authn.AuthContext{User: &authn.UserContext{OUHandle: ouHandle, Roles: roles}}
}
