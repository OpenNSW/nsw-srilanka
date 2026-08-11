// Package authn is this application's boundary onto the shared JWT
// authentication library, github.com/OpenNSW/core/authn.
//
// This is the ONLY package that may import core/authn. Handlers, domain
// services and the composition root all depend on the first-party types
// declared here instead. Two upstream API reshapes in quick succession each
// forced edits across five packages; funnelling the dependency through one
// package makes the next one a single-package change.
//
// It also owns the IdP claim vocabulary. The names of the claims this
// deployment reads are unexported constants here, declared to core/authn in
// config.go and flattened onto Principal, so no caller ever spells a claim
// name. Those names must stay in sync with the ThunderID token configuration
// under idp/resources/ — the same obligation internal/scopes carries for scope
// names.
package authn

import "context"

// IdP claim names read by this deployment. Adding one means declaring it in
// Config.coreConfig (or core/authn never extracts it) and surfacing it on
// Principal.
const (
	claimEmail       = "email"
	claimPhoneNumber = "phone_number"
	claimOUID        = "ouId"
	claimOUHandle    = "ouHandle"
)

// Kind distinguishes a human caller from a machine (M2M) client.
type Kind string

const (
	KindUser   Kind = "user"
	KindClient Kind = "client"
)

// Principal is this application's view of the authenticated caller for one
// request. Identity claims are flattened onto named fields so callers depend on
// this struct rather than on the IdP's claim spellings or on core/authn's
// context shape.
//
// Fields not applicable to a Kind are zero: a client principal has no OUHandle,
// a user principal has no ClientID.
type Principal struct {
	Kind Kind

	// UserID is the internally persisted user ID, resolved by
	// UserProfileService. Empty for client principals, and empty for a user
	// whose profile has not been resolved yet (see UserProfileService).
	UserID string
	// IDPUserID is the identity provider's ID for the user (the JWT "sub"
	// claim), which is not the same as UserID.
	IDPUserID string
	// ClientID identifies a machine client. Empty for user principals.
	ClientID string

	Roles  []string
	Scopes []string

	Email       string
	PhoneNumber string
	OUID        string
	OUHandle    string
}

// Subject returns a stable identifier for the principal: the persisted user ID
// (falling back to the IdP's user ID when the profile is not resolved) for
// users, the client ID for machine clients. Nil-safe.
func (p *Principal) Subject() string {
	if p == nil {
		return ""
	}
	switch p.Kind {
	case KindUser:
		if p.UserID != "" {
			return p.UserID
		}
		return p.IDPUserID
	case KindClient:
		return p.ClientID
	default:
		return ""
	}
}

// contextKey is unexported so a Principal can only reach a request context via
// ContextWithPrincipal, keeping this package the single writer.
type contextKey struct{}

// FromContext returns the Principal attached by the authentication middleware.
// ok is false for an unauthenticated request — a public route, a missing
// Authorization header, or middleware that was never applied.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(*Principal)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}

// ContextWithPrincipal attaches p to ctx. Production code should not call this
// directly — Manager.RequireAuthMiddleware does. It is exported for tests that
// need an authenticated request without minting a real token.
func ContextWithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
