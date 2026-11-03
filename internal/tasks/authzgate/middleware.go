// Package authzgate is Layer 1 of task-step authorization: an HTTP middleware for
// the task-write routes that reads the caller's identity and attaches an
// authz.Input — including a lazy case-ownership resolver — to the request
// context for the PRE_RESUME authz extension to evaluate.
//
// It resolves the principal eagerly (cheap) but defers ownership to a resolver
// the extension invokes only when a user rule matches by role, so clients,
// role-mismatches, and non-authz tasks incur no ownership lookup. A user owns a
// task's case in every role their OU holds on it (case_parties): in TNSW the OUs of
// the trader and CHA companies; in an agency, the agency's own OU. It depends only
// on a narrow interface (bootstrap injects the concrete repository), so the task
// HTTP surface, the authz policy evaluator, and the case-party domain each stay
// unaware of one another.
package authzgate

import (
	"context"
	"errors"
	"net/http"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/caseparty"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// CaseRoles reports the logical roles any of parties holds on a case — in TNSW, the
// consignment whose ID is the task's case ID. A case that does not exist, or that none
// of parties is on, yields an empty map, not an error. *caseparty.Repository satisfies
// it.
type CaseRoles interface {
	RolesOf(ctx context.Context, caseID string, parties ...caseparty.Ref) (map[string]bool, error)
}

// Middleware attaches the authz.Input (principal facts + a lazy ownership
// resolver) for task-write requests.
type Middleware struct {
	roles CaseRoles
}

// NewMiddleware builds the middleware. roles is required.
func NewMiddleware(roles CaseRoles) (*Middleware, error) {
	if roles == nil {
		return nil, errors.New("authzgate: case roles are required")
	}
	return &Middleware{roles: roles}, nil
}

// Handler wraps next (the task-write handler), attaching the authz Input.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if in, ok := m.resolve(r.Context()); ok {
			r = r.WithContext(taskauthz.WithInput(r.Context(), in))
		}
		next.ServeHTTP(w, r)
	})
}

// resolve builds the authz.Input from the request's auth context. ok is false for
// an unauthenticated request — no Input is attached and the extension denies 401.
func (m *Middleware) resolve(ctx context.Context) (taskauthz.Input, bool) {
	p, ok := authn.FromContext(ctx)
	if !ok {
		return taskauthz.Input{}, false
	}
	switch p.Kind {
	case authn.KindClient:
		return taskauthz.Input{Kind: taskauthz.KindClient, ClientID: p.ClientID}, true
	case authn.KindUser:
		return taskauthz.Input{
			Kind:       taskauthz.KindUser,
			Roles:      p.Roles,
			OwnedRoles: m.ownedRolesFor(p.OUHandle),
			UserID:     p.UserID,
		}, true
	default:
		return taskauthz.Input{}, false
	}
}

// ownedRolesFor returns a resolver bound to the caller's OU handle. The extension
// invokes it (with the task's case id) only when a user rule matches by
// role; that is the only point at which the DB is touched.
func (m *Middleware) ownedRolesFor(ouHandle string) taskauthz.OwnedRolesFunc {
	return func(ctx context.Context, caseID string) (map[string]bool, error) {
		// No task, so no case to own; no OU, so no party to hold a role on it.
		if caseID == "" || ouHandle == "" {
			return map[string]bool{}, nil
		}
		// The caller owns a case in every role their OU holds on it.
		return m.roles.RolesOf(ctx, caseID, caseparty.OU(ouHandle))
	}
}
