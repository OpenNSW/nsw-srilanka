// Package authn is this application's boundary onto the shared JWT
// authentication library, github.com/OpenNSW/core/authn.
//
// This is the ONLY package that may import core/authn. Handlers, domain
// services and the composition root all depend on the first-party types
// declared here instead, so an upstream API change touches only this package
// rather than every caller.
//
// It also owns the IdP claim vocabulary. The names of the claims every
// deployment reads are unexported constants here, declared to core/authn in
// config.go and flattened onto Principal, so no caller ever spells a claim
// name. Claims that differ per deployment (for attribute-based rules) are
// listed in config.yaml and reach callers through Principal.ExtraClaims. Both
// sets of names must stay in sync with the ThunderID token configuration in
// idp/bootstrap/ — the same obligation internal/scopes carries for scope names.
package authn

import (
	"context"
	"slices"
)

// IdP claim names read by every deployment. Adding one means declaring it in
// Config.coreConfig (or core/authn never extracts it), listing it in
// builtinUserClaims, and surfacing it on Principal.
const (
	claimEmail       = "email"
	claimPhoneNumber = "phone_number"
	claimOUID        = "ouId"
	claimOUHandle    = "ouHandle"
)

// builtinUserClaims are the claims Principal flattens onto named fields. They
// are reserved: a deployment can't also list one as an extra user claim, and
// Principal.ExtraClaims never repeats them.
var builtinUserClaims = []string{claimEmail, claimPhoneNumber, claimOUID, claimOUHandle}

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

	// ExtraClaims holds the claims this deployment lists under
	// authn.extraUserClaims (user principals) or authn.extraClientClaims
	// (client principals), keyed by claim name. A claim absent from the token is
	// absent here. The built-in claims above are never repeated in it. Nil when
	// none was configured or present.
	ExtraClaims Claims
}

// Claims is a set of token claims keyed by name, with the JSON-decoded value as
// the IdP sent it: string, []any, float64, bool or map[string]any. Treat it as
// read-only. A nil Claims is safe to read.
type Claims map[string]any

// String returns the claim as a string, or "" if it is absent or not a string.
func (c Claims) String(name string) string {
	v, _ := c[name].(string)
	return v
}

// Strings returns the claim as a string slice, or nil if it is absent or is not
// an array of strings. A single string is not split: the claim is read as it
// was issued, so a free-text attribute like "Customs Division" stays one value.
// Index the map directly for any other shape.
func (c Claims) Strings(name string) []string {
	switch v := c[name].(type) {
	case []string:
		return slices.Clone(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
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
