package agency

import (
	"context"
	"fmt"
	"net/http"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// RoleOfficer is the logical catalog role an agency officer acts in. A deployment
// whose catalog does not define it runs with the task authz gate unchanged.
const RoleOfficer = "officer"

// TaskGate is the shape of the task authz gate bootstrap wires onto the task routes;
// *authzgate.Middleware satisfies it.
type TaskGate interface {
	Handler(next http.Handler) http.Handler
}

// WrapTaskGate extends gate so that, besides the trader/CHA consignment ownership
// it already resolves, a caller owns the officer role on every task rooted in an
// injected workflow. The read and write evaluators then treat officers exactly like
// any other owner, so agency tasks go through the normal /api/v1/tasks/{id} routes
// with no second, unauthenticated surface.
//
// This re-attaches the Input the gate built, which the taskauthz.WithInput doc
// reserves for Layer 1. The wrapper is part of Layer 1 — it only runs behind the
// gate, and adds ownership rather than asserting a new identity.
func WrapTaskGate(gate TaskGate, repo Repository, roles map[string]string) TaskGate {
	if _, ok := roles[RoleOfficer]; !ok {
		return gate
	}
	return officerGate{gate: gate, repo: repo}
}

type officerGate struct {
	gate TaskGate
	repo Repository
}

func (g officerGate) Handler(next http.Handler) http.Handler {
	return g.gate.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if in, ok := taskauthz.InputFromContext(r.Context()); ok && in.Kind == taskauthz.KindUser {
			in.OwnedRoles = g.withOfficer(in.OwnedRoles)
			r = r.WithContext(taskauthz.WithInput(r.Context(), in))
		}
		next.ServeHTTP(w, r)
	}))
}

// withOfficer decorates base so the returned map also carries officer ownership.
// Token-role checks stay in taskauthz.Eligible: this only answers "is this task an
// agency task", and only once the caller is known to hold a relevant role.
func (g officerGate) withOfficer(base taskauthz.OwnedRolesFunc) taskauthz.OwnedRolesFunc {
	return func(ctx context.Context, rootWorkflowID string) (map[string]bool, error) {
		owned := map[string]bool{}
		if base != nil {
			var err error
			if owned, err = base(ctx, rootWorkflowID); err != nil {
				return nil, err
			}
		}
		if rootWorkflowID == "" {
			return owned, nil
		}
		injected, err := g.repo.Exists(ctx, rootWorkflowID)
		if err != nil {
			return nil, fmt.Errorf("agency: resolve officer ownership: %w", err)
		}
		owned[RoleOfficer] = injected
		return owned, nil
	}
}
