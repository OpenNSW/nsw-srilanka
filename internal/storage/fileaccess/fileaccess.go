// Package fileaccess decides who a file token is for, what it allows and how
// long it lasts.
//
// Callers outside the server never see a stored file's value, only a token
// for it. A token names its holder (a user by IdP subject, a machine client by
// client ID) and lists its scopes, the actions it allows:
//
//   - ScopeRead: open the file on the storage route.
//   - ScopeSubmit: send the token back as a file field's value in a task
//     submission, where it resolves to the stored value.
//
// A user's token that can read expires after the configured lifetime. One that
// can only be submitted opens nothing, so it never expires. A machine client's
// token never expires either, because an OGA keeps it in its own records for as
// long as it needs the file.
//
// Tokens are grants. Resolve checks only that the caller is the token's
// holder, that the token allows what the caller is doing and, if it expires,
// that it hasn't; it doesn't check again whether the caller may still see the
// file. So issue a token only to a caller already allowed what its scopes
// allow. A caller who loses that access keeps it until their tokens expire:
// for a user, after the configured lifetime; otherwise, once the key that
// sealed them is retired.
//
// A deployment's tokens are the only ones it can open. A machine client in the
// catalog's clientToService may also send a value that is not one of them: it is
// a token its own service issued, and Accept returns it as that service's file
// (see fileref). The service comes from the authenticated client, never from
// the value.
//
// filetoken seals and opens the tokens. This package only binds them to
// callers.
package fileaccess

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileref"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
)

// Scope is an action a file token allows its holder.
type Scope string

const (
	// ScopeRead allows opening the file on the storage route.
	ScopeRead Scope = "read"
	// ScopeSubmit allows sending the token as a file field's value in a task
	// submission.
	ScopeSubmit Scope = "submit"
)

var (
	// ErrInvalid means the reference is not a file token this deployment
	// issued.
	ErrInvalid = errors.New("fileaccess: not a file token")
	// ErrNotYours means the token was issued to another caller.
	ErrNotYours = errors.New("fileaccess: file token issued to another caller")
	// ErrScope means the token was the caller's but doesn't allow what the
	// caller is doing with it.
	ErrScope = errors.New("fileaccess: file token doesn't allow this")
	// ErrExpired means the token was the caller's but has expired.
	ErrExpired = errors.New("fileaccess: file token expired")
	// ErrNoService means a machine client sent a value that is not one of this
	// deployment's tokens, and the catalog does not say which service the client
	// is, so the file has no owner to open it through.
	ErrNoService = errors.New("fileaccess: the client is not a known service")
	// ErrTooLong means a token for a peer would be longer than the peer accepts.
	ErrTooLong = errors.New("fileaccess: file token too long for a peer")
)

// Access issues file tokens to callers and resolves them back to the
// resources they stand for.
type Access struct {
	codec      *filetoken.Codec
	userTTL    time.Duration
	serviceFor func(clientID string) (string, bool)
}

// Option configures an Access.
type Option func(*Access)

// WithClientServices says which service each machine client is, so that Accept
// can store the files a client sends as that service's. serviceFor returns the
// service a client ID is, or false for a client that may not send files.
func WithClientServices(serviceFor func(clientID string) (service string, ok bool)) Option {
	return func(a *Access) { a.serviceFor = serviceFor }
}

// New returns an Access that seals with codec and gives users tokens that
// can read a lifetime of userTTL.
func New(codec *filetoken.Codec, userTTL time.Duration, opts ...Option) (*Access, error) {
	if codec == nil {
		return nil, errors.New("fileaccess: codec is required")
	}
	if userTTL <= 0 {
		return nil, errors.New("fileaccess: user token lifetime must be positive")
	}
	a := &Access{codec: codec, userTTL: userTTL}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// Issue returns a token that lets p, the caller, do what scopes allow with
// resource.
func (a *Access) Issue(p *authn.Principal, resource string, scopes ...Scope) (string, error) {
	holder := holderOf(p)
	if holder == "" {
		return "", errors.New("fileaccess: the caller has no subject or client ID to issue a token to")
	}
	return a.issue(holder, p.Kind == authn.KindUser, resource, scopes)
}

// IssueForClient returns a token that lets the machine client clientID do
// what scopes allow with resource, when that client is not the caller. It is
// how a file is handed to an OGA: the push runs in a workflow step, with no
// request principal, and names its recipient from the catalog. The token is
// for a peer, which accepts none longer than fileref.MaxValueLength, so a
// longer one is ErrTooLong.
func (a *Access) IssueForClient(clientID, resource string, scopes ...Scope) (string, error) {
	if clientID == "" {
		return "", errors.New("fileaccess: client ID is required")
	}
	token, err := a.issue(clientHolder(clientID), false, resource, scopes)
	if err != nil {
		return "", err
	}
	if len(token) > fileref.MaxValueLength {
		return "", fmt.Errorf("%w: %d bytes, a peer accepts at most %d", ErrTooLong, len(token), fileref.MaxValueLength)
	}
	return token, nil
}

// issue seals a token for holder. A user's token that can read expires after
// the user lifetime; any other never does.
func (a *Access) issue(holder string, user bool, resource string, scopes []Scope) (string, error) {
	if len(scopes) == 0 {
		return "", errors.New("fileaccess: a token needs at least one scope")
	}
	names := make([]string, len(scopes))
	var lifetime time.Duration
	for i, scope := range scopes {
		names[i] = string(scope)
		if user && scope == ScopeRead {
			lifetime = a.userTTL
		}
	}
	token, err := a.codec.Seal(filetoken.Grant{Resource: resource, Holder: holder, Scopes: names}, lifetime)
	if err != nil {
		return "", fmt.Errorf("fileaccess: %w", err)
	}
	return token, nil
}

// Resolve returns the resource ref stands for, so that p can do what need
// allows with it, provided ref is a token issued to p, allows need and has not
// expired. It checks nothing else (see the package doc). A token issued before
// tokens carried scopes allows nothing, so it is ErrScope: refused, rather
// than taken for a value that is not a token. The checks run in
// that order: a token issued to someone else is ErrNotYours whatever it
// allows, and even once expired, so the caller is not told to fetch a fresh
// one it could never get.
func (a *Access) Resolve(p *authn.Principal, ref string, need Scope) (string, error) {
	claims, err := a.codec.Open(ref)
	if err != nil && !errors.Is(err, filetoken.ErrExpired) {
		return "", ErrInvalid
	}
	return a.check(p, claims, err, need)
}

// Accept returns the value to store for ref, a file field's value in a
// submission or an inject sent by p. A token this deployment issued to p that
// allows submit resolves as Resolve does, and is refused, never taken for
// another service's, when it is someone else's, lacks submit or has expired.
//
// Any other value is not this deployment's. From a machine client the catalog
// lists as a service, it is a token that service issued, and is returned as
// that service's file: the service comes from p alone. From a user, or a
// client the catalog does not list, it is ErrInvalid or ErrNoService.
func (a *Access) Accept(p *authn.Principal, ref string) (string, error) {
	claims, err := a.codec.Open(ref)
	if err == nil || errors.Is(err, filetoken.ErrExpired) {
		return a.check(p, claims, err, ScopeSubmit)
	}
	if p == nil || p.Kind != authn.KindClient {
		return "", ErrInvalid
	}
	var (
		service string
		ok      bool
	)
	if a.serviceFor != nil {
		service, ok = a.serviceFor(p.ClientID)
	}
	if !ok {
		return "", ErrNoService
	}
	foreign, err := fileref.Foreign(service, ref)
	if err != nil {
		return "", ErrInvalid
	}
	return foreign.String(), nil
}

// check decides whether p may do need with the token claims came from, which
// opened with openErr (nil, or filetoken.ErrExpired).
func (a *Access) check(p *authn.Principal, claims filetoken.Claims, openErr error, need Scope) (string, error) {
	if holder := holderOf(p); holder == "" || claims.Holder != holder {
		return "", ErrNotYours
	}
	if !slices.Contains(claims.Scopes, string(need)) {
		return "", ErrScope
	}
	if openErr != nil {
		return "", ErrExpired
	}
	return claims.Resource, nil
}

// holderOf names p as a token holder, or returns "" when p has no identity a
// token can name. A user is named by the IdP subject rather than the persisted
// user ID, which is not resolved on every request.
func holderOf(p *authn.Principal) string {
	if p == nil {
		return ""
	}
	switch p.Kind {
	case authn.KindUser:
		if p.IDPUserID != "" {
			return userHolder(p.IDPUserID)
		}
	case authn.KindClient:
		if p.ClientID != "" {
			return clientHolder(p.ClientID)
		}
	}
	return ""
}

// The prefixes keep a user and a machine client with the same ID apart.
func userHolder(sub string) string        { return "u:" + sub }
func clientHolder(clientID string) string { return "c:" + clientID }
