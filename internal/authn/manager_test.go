package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreauthn "github.com/OpenNSW/core/authn"
)

func TestNewManager_InvalidConfig(t *testing.T) {
	if _, err := NewManager(nil, Config{}); err == nil {
		t.Fatal("expected an error for an empty config")
	}
}

func TestNewManager_AllowsNilUserProfileService(t *testing.T) {
	m, err := NewManager(nil, validConfig())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Health(); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// attachPrincipal is the second half of RequireAuthMiddleware: it converts
// core/authn's request context into a Principal under this package's key, so
// handlers read identity through FromContext and never touch a core type.
func TestAttachPrincipal(t *testing.T) {
	t.Run("converts core auth context", func(t *testing.T) {
		var got *Principal
		var ok bool
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got, ok = FromContext(r.Context())
		})

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), coreauthn.AuthContextKey, &coreauthn.AuthContext{
			User: &coreauthn.UserContext{
				ID:          "u1",
				Roles:       []string{"Trader"},
				ExtraClaims: coreauthn.ExtraClaims{"ouHandle": "ou-001"},
			},
		}))

		attachPrincipal(next).ServeHTTP(httptest.NewRecorder(), r)

		if !ok {
			t.Fatal("expected a principal to be attached")
		}
		if got.UserID != "u1" || got.OUHandle != "ou-001" || got.Kind != KindUser {
			t.Fatalf("unexpected principal: %#v", got)
		}
	})

	// Without a core auth context there is nothing to convert; the request must
	// proceed unattached so downstream checks deny it, not panic.
	t.Run("passes through an unauthenticated request", func(t *testing.T) {
		called := false
		attached := true
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			_, attached = FromContext(r.Context())
		})

		attachPrincipal(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

		if !called {
			t.Fatal("expected the next handler to run")
		}
		if attached {
			t.Fatal("expected no principal to be attached")
		}
	})
}

// RequireAuthMiddleware composes core's enforcement with attachPrincipal, so an
// unauthenticated request is rejected before reaching the handler.
func TestManager_RequireAuthMiddleware_RejectsUnauthenticated(t *testing.T) {
	m, err := NewManager(nil, validConfig())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	called := false
	protected := m.RequireAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("expected the protected handler not to run")
	}
}
