// Package fileaccess decides who a file token is for and how long it lasts.
//
// Callers outside the server never see a stored file's value, only a token
// for it. A user's token names that user and expires after the configured
// lifetime: portals hold it in task data and present it to open the file. A
// machine client's token names the client and never expires, because an OGA
// keeps it in its own records for as long as it needs the file.
//
// Tokens are grants. Resolve checks only that the caller is the token's holder
// and, for a user, that it has not expired; it does not check again whether
// the caller may still see the file. So issue a token only to a caller already
// allowed to see what it stands for. A caller who loses that access keeps it
// until their tokens expire: for a user, after the configured lifetime; for a
// machine client, once the key that sealed them is retired.
//
// filetoken seals and opens the tokens. This package only binds them to
// callers.
package fileaccess

import (
	"errors"
	"fmt"
	"time"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
)

var (
	// ErrInvalid means the reference is not a file token this deployment
	// issued.
	ErrInvalid = errors.New("fileaccess: not a file token")
	// ErrNotYours means the token was issued to another caller.
	ErrNotYours = errors.New("fileaccess: file token issued to another caller")
	// ErrExpired means the token was the caller's but has expired.
	ErrExpired = errors.New("fileaccess: file token expired")
)

// Access issues file tokens to callers and resolves them back to the
// resources they stand for.
type Access struct {
	codec   *filetoken.Codec
	userTTL time.Duration
}

// New returns an Access that seals with codec and gives users tokens that
// last userTTL.
func New(codec *filetoken.Codec, userTTL time.Duration) (*Access, error) {
	if codec == nil {
		return nil, errors.New("fileaccess: codec is required")
	}
	if userTTL <= 0 {
		return nil, errors.New("fileaccess: user token lifetime must be positive")
	}
	return &Access{codec: codec, userTTL: userTTL}, nil
}

// IssueFor returns a token that lets p, the caller, use resource: a user's
// token expires after the user lifetime, a machine client's never does.
func (a *Access) IssueFor(p *authn.Principal, resource string) (string, error) {
	holder := holderOf(p)
	if holder == "" {
		return "", errors.New("fileaccess: the caller has no subject or client ID to issue a token to")
	}
	var lifetime time.Duration
	if p.Kind == authn.KindUser {
		lifetime = a.userTTL
	}
	return a.seal(resource, holder, lifetime)
}

// IssueForClient returns a token that lets the machine client clientID use
// resource, when that client is not the caller. It is how a file is handed to
// an OGA: the push runs in a workflow step, with no request principal, and
// names its recipient from the catalog. The token never expires.
func (a *Access) IssueForClient(clientID, resource string) (string, error) {
	if clientID == "" {
		return "", errors.New("fileaccess: client ID is required")
	}
	return a.seal(resource, clientHolder(clientID), 0)
}

// Resolve returns the resource ref stands for, provided ref is a token issued
// to p and, for a user, has not expired. It checks nothing else (see the
// package doc). A token issued to someone else is ErrNotYours even once
// expired, so the caller is not told to fetch a fresh one it could never get.
func (a *Access) Resolve(p *authn.Principal, ref string) (string, error) {
	claims, err := a.codec.Open(ref)
	if err != nil && !errors.Is(err, filetoken.ErrExpired) {
		return "", ErrInvalid
	}
	if holder := holderOf(p); holder == "" || claims.Holder != holder {
		return "", ErrNotYours
	}
	if err != nil {
		return "", ErrExpired
	}
	return claims.Resource, nil
}

func (a *Access) seal(resource, holder string, lifetime time.Duration) (string, error) {
	token, err := a.codec.Seal(resource, holder, lifetime)
	if err != nil {
		return "", fmt.Errorf("fileaccess: %w", err)
	}
	return token, nil
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
