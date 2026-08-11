package authn

import (
	"context"
	"testing"

	coreauthn "github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/authz"
)

// ScopePrincipal must satisfy core/authz's Principal interface structurally.
// This assertion is why internal/authn can own the identity type without
// importing core/authz in non-test code, and why the composition root can keep
// building the Authorizer. Breaking it breaks every scope-gated route.
var _ authz.Principal = ScopePrincipal{}

func TestFromContext_Unauthenticated(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("expected no principal on a bare context")
	}
	// A nil principal stored explicitly must still read as unauthenticated,
	// rather than handing callers a nil pointer to dereference.
	if _, ok := FromContext(ContextWithPrincipal(context.Background(), nil)); ok {
		t.Fatal("expected a nil principal to read as unauthenticated")
	}
}

func TestContextWithPrincipal_RoundTrip(t *testing.T) {
	want := &Principal{Kind: KindUser, UserID: "u1", OUHandle: "ou-1"}
	got, ok := FromContext(ContextWithPrincipal(context.Background(), want))
	if !ok {
		t.Fatal("expected a principal")
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrincipal_Subject(t *testing.T) {
	tests := []struct {
		name string
		p    *Principal
		want string
	}{
		{"nil principal", nil, ""},
		{"user with resolved id", &Principal{Kind: KindUser, UserID: "u1", IDPUserID: "idp1"}, "u1"},
		{"user falls back to idp id", &Principal{Kind: KindUser, IDPUserID: "idp1"}, "idp1"},
		{"client", &Principal{Kind: KindClient, ClientID: "SVC"}, "SVC"},
		{"unknown kind", &Principal{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Subject(); got != tt.want {
				t.Fatalf("Subject() = %q, want %q", got, tt.want)
			}
		})
	}
}

// principalFromAuthContext is the conversion the middleware runs once per
// request, and the only place the IdP's claim names are read.
func TestPrincipalFromAuthContext_UserFlattensDeclaredClaims(t *testing.T) {
	p := principalFromAuthContext(&coreauthn.AuthContext{
		User: &coreauthn.UserContext{
			ID:        "u1",
			IDPUserID: "idp1",
			Roles:     []string{"Trader"},
			Scopes:    []string{"nsw:task:read"},
			ExtraClaims: coreauthn.ExtraClaims{
				"email":        "trader@example.com",
				"phone_number": "+94771234567",
				"ouId":         "OU-001",
				"ouHandle":     "ou-001",
			},
		},
	})
	if p == nil {
		t.Fatal("expected a principal")
	}
	if p.Kind != KindUser || p.UserID != "u1" || p.IDPUserID != "idp1" {
		t.Fatalf("unexpected identity: %#v", p)
	}
	if p.Email != "trader@example.com" || p.PhoneNumber != "+94771234567" ||
		p.OUID != "OU-001" || p.OUHandle != "ou-001" {
		t.Fatalf("unexpected claims: %#v", p)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "Trader" {
		t.Fatalf("unexpected roles: %#v", p.Roles)
	}
	if len(p.Scopes) != 1 || p.Scopes[0] != "nsw:task:read" {
		t.Fatalf("unexpected scopes: %#v", p.Scopes)
	}
}

func TestPrincipalFromAuthContext_UserWithoutExtraClaims(t *testing.T) {
	p := principalFromAuthContext(&coreauthn.AuthContext{
		User: &coreauthn.UserContext{ID: "u1"},
	})
	if p == nil {
		t.Fatal("expected a principal")
	}
	if p.Email != "" || p.PhoneNumber != "" || p.OUID != "" || p.OUHandle != "" {
		t.Fatalf("expected empty claim fields, got %#v", p)
	}
}

func TestPrincipalFromAuthContext_ClientCarriesNoIdentityClaims(t *testing.T) {
	p := principalFromAuthContext(&coreauthn.AuthContext{
		Client: &coreauthn.ClientContext{
			ClientID: "GOVPAY_TO_NSW",
			Roles:    []string{"PaymentWebhookM2M"},
			Scopes:   []string{"nsw:payment-webhooks:process"},
		},
	})
	if p == nil {
		t.Fatal("expected a principal")
	}
	if p.Kind != KindClient || p.ClientID != "GOVPAY_TO_NSW" {
		t.Fatalf("unexpected client principal: %#v", p)
	}
	if p.OUHandle != "" || p.Email != "" {
		t.Fatalf("client principal should carry no user claims: %#v", p)
	}
}

// Fail closed: a context carrying neither principal must not produce a
// principal, so downstream scope and ownership checks deny the request.
func TestPrincipalFromAuthContext_EmptyYieldsNoPrincipal(t *testing.T) {
	if p := principalFromAuthContext(nil); p != nil {
		t.Fatalf("expected nil for a nil auth context, got %#v", p)
	}
	if p := principalFromAuthContext(&coreauthn.AuthContext{}); p != nil {
		t.Fatalf("expected nil for an empty auth context, got %#v", p)
	}
}

func TestScopePrincipalFromContext(t *testing.T) {
	if _, ok := ScopePrincipalFromContext(context.Background()); ok {
		t.Fatal("expected no scope principal on an unauthenticated context")
	}

	ctx := ContextWithPrincipal(context.Background(), &Principal{
		Kind:   KindUser,
		UserID: "u1",
		Roles:  []string{"Trader"},
		Scopes: []string{"nsw:consignment:read"},
	})
	sp, ok := ScopePrincipalFromContext(ctx)
	if !ok {
		t.Fatal("expected a scope principal")
	}
	if sp.Subject() != "u1" {
		t.Fatalf("Subject() = %q, want u1", sp.Subject())
	}
	if len(sp.Roles()) != 1 || sp.Roles()[0] != "Trader" {
		t.Fatalf("Roles() = %#v", sp.Roles())
	}
	if len(sp.Scopes()) != 1 || sp.Scopes()[0] != "nsw:consignment:read" {
		t.Fatalf("Scopes() = %#v", sp.Scopes())
	}
}
