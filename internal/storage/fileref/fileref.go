// Package fileref is the format of a stored file value: the string a task's
// data holds for a file field.
//
// A deployment owns the files uploaded to it, and a stored value says which
// deployment owns the file:
//
//   - a bare storage key (such as "3f2b9c1e-….pdf") is a file this deployment
//     owns;
//   - "<service>:<token>" is a file another service owns. service is that
//     peer's id in the services registry, and token is the opaque token the
//     peer issued to this deployment's client.
//
// core/storage keys never contain ':', so the two forms cannot be confused,
// and core's key check refuses any foreign value: a foreign value is
// dispatched to its owner before storage is called.
//
// The package is pure: it does no I/O and knows nothing of who may read a
// file or how a peer is reached.
package fileref

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MaxValueLength is the longest token a foreign value may carry, and so the
// longest token this deployment hands to a peer, which accepts no more.
const MaxValueLength = 2048

// ErrInvalid means a service or token is not one a foreign value may hold.
var ErrInvalid = errors.New("fileref: not a valid foreign file reference")

var (
	namespacePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,62}$`)
	tokenPattern     = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Ref names a stored file and the service that owns it.
type Ref struct {
	// Namespace is the owning service's id, or "" when this deployment owns the
	// file.
	Namespace string
	// Value is the storage key of an own file, or the token the owning service
	// issued for a foreign one.
	Value string
}

// Own returns the reference to a file this deployment owns, stored under key.
func Own(key string) Ref {
	return Ref{Value: key}
}

// Foreign returns the reference to the file the service namespace owns, which
// token opens. It is an error unless both are well formed: a token is
// 1 to MaxValueLength characters of letters, digits, '.', '_' and '-', and not
// exactly "." or "..", so it is one safe path segment. (A compact JWE token
// contains "..", which is fine inside a segment.)
func Foreign(namespace, token string) (Ref, error) {
	if !ValidNamespace(namespace) {
		return Ref{}, fmt.Errorf("%w: service id", ErrInvalid)
	}
	if !validToken(token) {
		return Ref{}, fmt.Errorf("%w: token", ErrInvalid)
	}
	return Ref{Namespace: namespace, Value: token}, nil
}

// ValidNamespace reports whether s can name a service in a foreign value: a
// letter followed by up to 62 letters, digits, '_' or '-'.
func ValidNamespace(s string) bool {
	return namespacePattern.MatchString(s)
}

func validToken(s string) bool {
	return len(s) > 0 && len(s) <= MaxValueLength && s != "." && s != ".." && tokenPattern.MatchString(s)
}

// Parse returns the reference a stored value stands for. It never fails:
// anything that is not a well-formed "<service>:<token>" is taken as an own
// key, which storage then refuses if it is not one.
func Parse(stored string) Ref {
	namespace, token, found := strings.Cut(stored, ":")
	if !found {
		return Own(stored)
	}
	ref, err := Foreign(namespace, token)
	if err != nil {
		return Own(stored)
	}
	return ref
}

// IsOwn reports whether this deployment owns the file.
func (r Ref) IsOwn() bool {
	return r.Namespace == ""
}

// String returns the stored value: the bare key of an own file, otherwise
// "<service>:<token>".
func (r Ref) String() string {
	if r.IsOwn() {
		return r.Value
	}
	return r.Namespace + ":" + r.Value
}
