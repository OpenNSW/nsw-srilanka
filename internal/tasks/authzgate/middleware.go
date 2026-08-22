// Package authzgate is Layer 1 of task-step authorization: an HTTP middleware for
// the task-write routes that reads the caller's identity and attaches an
// authz.Input — including a lazy consignment-ownership resolver — to the request
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
	"net/http"

	"github.com/OpenNSW/core/authn"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// Logical role names ownedRolesFor resolves ownership for. The global catalog
// must define both — NewMiddleware enforces it.
const (
	roleTrader = "trader"
	roleCHA    = "cha"
)

// OwnershipResolver returns the trader and CHA company ids that own a
// consignment. *consignment.Service satisfies it via GetOwnership.
type OwnershipResolver interface {
	GetOwnership(ctx context.Context, consignmentID string) (traderCompanyID, chaCompanyID string, err error)
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
	ownership OwnershipResolver
	company   CompanyResolver
}

// NewMiddleware builds the middleware. ownership and company are required;
// roles is the global catalog's Roles map, validated here so a catalog missing
// a role ownedRolesFor depends on fails construction instead of silently
// denying every request in that role.
func NewMiddleware(ownership OwnershipResolver, company CompanyResolver, roles map[string]string) (*Middleware, error) {
	if err := catalog.RequireRoles(roles, roleTrader, roleCHA); err != nil {
		return nil, err
	}
	return &Middleware{ownership: ownership, company: company}, nil
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
	ac := authn.GetAuthContext(ctx)
	if ac == nil {
		return taskauthz.Input{}, false
	}
	switch ac.Type() {
	case authn.ClientPrincipalType:
		if ac.Client == nil {
			return taskauthz.Input{}, false
		}
		return taskauthz.Input{Kind: taskauthz.KindClient, ClientID: ac.Client.ClientID}, true
	case authn.UserPrincipalType:
		if ac.User == nil {
			return taskauthz.Input{}, false
		}
		return taskauthz.Input{
			Kind:       taskauthz.KindUser,
			Roles:      ac.User.Roles,
			OwnedRoles: m.ownedRolesFor(ac.User.OUHandle),
		}, true
	default:
		return taskauthz.Input{}, false
	}
}

// ownedRolesFor returns a resolver bound to the caller's OU handle. The extension
// invokes it (with the task's root workflow id) only when a user rule matches by
// role; that is the only point at which the DB is touched.
func (m *Middleware) ownedRolesFor(ouHandle string) taskauthz.OwnedRolesFunc {
	return func(ctx context.Context, rootWorkflowID string) (map[string]bool, error) {
		owned := map[string]bool{}
		// Nothing (or nobody) to match against: no task, or a caller with no OU
		// handle. Skip the lookups entirely.
		if rootWorkflowID == "" || ouHandle == "" {
			return owned, nil
		}
		userCompanyID, err := m.company.CompanyIDByOUHandle(ctx, ouHandle)
		if err != nil {
			return nil, err
		}
		if userCompanyID == "" {
			return owned, nil // no company profile — owns nothing; skip ownership lookup
		}
		traderCompanyID, chaCompanyID, err := m.ownership.GetOwnership(ctx, rootWorkflowID)
		if err != nil {
			return nil, err
		}
		// roleTrader/roleCHA are hardcoded because each maps to a specific
		// ownership column here, not because they are arbitrary; NewMiddleware's
		// validateCatalogRoles call enforces that the catalog defines both.
		owned[roleTrader] = userCompanyID == traderCompanyID
		owned[roleCHA] = chaCompanyID != "" && userCompanyID == chaCompanyID
		return owned, nil
	}
}
