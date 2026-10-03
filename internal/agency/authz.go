package agency

import (
	"context"
	"fmt"
	"net/http"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// RoleOfficer is the logical catalog role an agency officer acts in. An agency
// deployment's catalog must define it.
const RoleOfficer = "officer"

// OfficerGate is Layer 1 of task authorization in agency mode, in place of TNSW's
// authzgate. It attaches the same taskauthz.Input, but the only ownership it resolves
// is officer ownership of injected workflows: an agency has no consignments, so there
// is no trader or CHA company to look up. The read and write evaluators then treat
// officers like any other owner, and agency tasks go through the normal
// /api/v1/tasks/{id} routes.
type OfficerGate struct {
	repo Repository
}

// NewOfficerGate creates an OfficerGate.
func NewOfficerGate(repo Repository) *OfficerGate {
	return &OfficerGate{repo: repo}
}

// Handler wraps next, attaching the caller's taskauthz.Input. An unauthenticated
// request gets none, which the evaluators deny.
func (g *OfficerGate) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if in, ok := g.resolve(r.Context()); ok {
			r = r.WithContext(taskauthz.WithInput(r.Context(), in))
		}
		next.ServeHTTP(w, r)
	})
}

func (g *OfficerGate) resolve(ctx context.Context) (taskauthz.Input, bool) {
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

// ownedRoles reports officer ownership of the task's root workflow: true when it is an
// injected workflow. Token-role checks stay in taskauthz.Eligible, which calls this
// only once the caller is known to hold a relevant role.
func (g *OfficerGate) ownedRoles(ctx context.Context, rootWorkflowID string) (map[string]bool, error) {
	if rootWorkflowID == "" {
		return map[string]bool{}, nil
	}
	injected, err := g.repo.Exists(ctx, rootWorkflowID)
	if err != nil {
		return nil, fmt.Errorf("agency: resolve officer ownership: %w", err)
	}
	return map[string]bool{RoleOfficer: injected}, nil
}
