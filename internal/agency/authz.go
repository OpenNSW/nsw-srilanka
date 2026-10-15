package agency

import (
	"context"
	"fmt"
	"net/http"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// RoleGate is Layer 1 of task authorization in agency mode, in place of TNSW's
// authzgate. It attaches the same taskauthz.Input, but the only ownership it resolves
// is that of injected workflows, in every role of the agency's catalog: an agency has
// no consignments, so there is no trader or CHA company to look up, and which of those
// roles may act on a task is up to the task's own authz and read rules. The read and
// write evaluators then treat officers like any other owner, and agency tasks go
// through the normal /api/v1/tasks/{id} routes.
type RoleGate struct {
	repo  Repository
	roles []string // the catalog's logical role names
}

// NewRoleGate creates an RoleGate that reports ownership in each of roles, the
// logical role names of the agency's catalog.
func NewRoleGate(repo Repository, roles []string) *RoleGate {
	return &RoleGate{repo: repo, roles: roles}
}

// Handler wraps next, attaching the caller's taskauthz.Input. An unauthenticated
// request gets none, which the evaluators deny.
func (g *RoleGate) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if in, ok := g.resolve(r.Context()); ok {
			r = r.WithContext(taskauthz.WithInput(r.Context(), in))
		}
		next.ServeHTTP(w, r)
	})
}

func (g *RoleGate) resolve(ctx context.Context) (taskauthz.Input, bool) {
	p, ok := authn.FromContext(ctx)
	if !ok {
		return taskauthz.Input{}, false
	}
	switch p.Kind {
	case authn.KindClient:
		return taskauthz.Input{Kind: taskauthz.KindClient, ClientID: p.ClientID}, true
	case authn.KindUser:
		return taskauthz.Input{Kind: taskauthz.KindUser, Roles: p.Roles, OwnedRoles: g.ownedRoles}, true
	default:
		return taskauthz.Input{}, false
	}
}

// ownedRoles reports ownership of the task's case in every catalog role: true when it
// is an injected workflow, which is its own case (see Workflow). Token-role checks stay
// in taskauthz.Eligible, which calls this only once the caller is known to hold a
// relevant role.
func (g *RoleGate) ownedRoles(ctx context.Context, caseID string) (map[string]bool, error) {
	owned := make(map[string]bool, len(g.roles))
	if caseID == "" {
		return owned, nil
	}
	injected, err := g.repo.Exists(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("agency: resolve ownership: %w", err)
	}
	for _, name := range g.roles {
		owned[name] = injected
	}
	return owned, nil
}
