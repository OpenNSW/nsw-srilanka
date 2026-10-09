// Package fileaccess decides who a file token is for and how long it lasts.
//
// Callers outside the server never see a stored file's value, only a token
// for it. A user's token names that user and expires after the configured
// lifetime: portals hold it in task data and present it to open the file. A
// machine client's token names the client and never expires, because an OGA
// keeps it in its own records for as long as it needs the file.
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

// Access issues file tokens to callers and resolves them back to stored
// values.
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

// IssueFor returns a token that lets p use value: a user's token expires after
// the user lifetime, a machine client's never does.
func (a *Access) IssueFor(p *authn.Principal, value string) (string, error) {
	holder := holderOf(p)
	if holder == "" {
		return "", errors.New("fileaccess: the caller has no subject or client ID to issue a token to")
	}
	var lifetime time.Duration
	if p.Kind == authn.KindUser {
		lifetime = a.userTTL
	}
	return a.seal(value, holder, lifetime)
}

// IssueForClient returns a token that lets the machine client clientID use
// value, without the client being the caller: it is how a stored value is
// handed to an OGA. The token never expires.
func (a *Access) IssueForClient(clientID, value string) (string, error) {
	if clientID == "" {
		return "", errors.New("fileaccess: client ID is required")
	}
	return a.seal(value, clientHolder(clientID), 0)
}

// Resolve returns the stored value ref stands for, provided ref is a token
// issued to p and, for a user, has not expired. A token issued to someone else
// is ErrNotYours even once expired, so the caller is not told to fetch a fresh
// one it could never get.
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
	return claims.Value, nil
}

func (a *Access) seal(value, holder string, lifetime time.Duration) (string, error) {
	token, err := a.codec.Seal(value, holder, lifetime)
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
