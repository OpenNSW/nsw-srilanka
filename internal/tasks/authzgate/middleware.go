// Package authzgate is Layer 1 of task-step authorization: an HTTP middleware for
// the task-write routes that reads the caller's identity and attaches an
// authz.Input — including a lazy case-ownership resolver — to the request
// context for the PRE_RESUME authz extension to evaluate.
//
// It resolves the principal eagerly (cheap) but defers ownership to a resolver
// the extension invokes only when a user rule matches by role, so clients,
// role-mismatches, and non-authz tasks incur no ownership lookup. It depends only
// on narrow interfaces (bootstrap injects the concrete services), so the task
// HTTP surface, the authz policy evaluator, and the consignment/company domains
// each stay unaware of one another.
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

// CompanyResolver resolves a user's company id from their OU handle. It must
// return ("", nil) — not an error — when the user has no company profile, so a
// missing profile denies cleanly rather than surfacing as a 500.
type CompanyResolver interface {
	CompanyIDByOUHandle(ctx context.Context, ouHandle string) (string, error)
}

// Middleware attaches the authz.Input (principal facts + a lazy ownership
// resolver) for task-write requests.
type Middleware struct {
	roles   CaseRoles
	company CompanyResolver
}

// NewMiddleware builds the middleware. roles and company are both required.
func NewMiddleware(roles CaseRoles, company CompanyResolver) (*Middleware, error) {
	if roles == nil || company == nil {
		return nil, errors.New("authzgate: case roles and company resolver are required")
	}
	return &Middleware{roles: roles, company: company}, nil
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
		owned := map[string]bool{}
		// Nothing (or nobody) to match against: no task, or a caller with no OU
		// handle. Skip the lookups entirely.
		if caseID == "" || ouHandle == "" {
			return owned, nil
		}
		userCompanyID, err := m.company.CompanyIDByOUHandle(ctx, ouHandle)
		if err != nil {
			return nil, err
		}
		if userCompanyID == "" {
			return owned, nil // no company profile — owns nothing; skip ownership lookup
		}
		// The caller owns a case in every role their company holds on it.
		return m.roles.RolesOf(ctx, caseID, caseparty.Company(userCompanyID))
	}
}
