// Package authz provides nsw-srilanka's task-step authorization extension: a
// PRE_RESUME policy gate that decides whether the caller may run a command on a
// task at its current state.
//
// It is a pure evaluator. Layer 1 (internal/tasks/authzgate) resolves the
// caller's identity and attaches it — together with a lazy ownership resolver —
// to the request context as a taskauthz.Input; this extension only matches that
// against the per-task policy and the catalog it is handed at construction. It
// reads no configuration file, resolves ownership only when a user rule actually
// needs it, and never touches domain services directly.
//
// Reads are guarded separately, by internal/tasks/readauthz — core has no read
// hook, so only the write path can be an extension. Both share the principal
// types and the eligibility rule in internal/tasks/taskauthz.
package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// Sentinel errors mapped to HTTP status by the task handler. errors.Is still
// matches after the orchestrator wraps them with %w; any other error is a 500.
var (
	ErrUnauthenticated = errors.New("task authz: unauthenticated")
	ErrForbidden       = errors.New("task authz: forbidden")
)

// Extension is the PRE_RESUME task extension enforcing, per task state and
// command, which principals may advance a task: users by token role + resolved
// ownership, M2M clients by client id. It is deny-by-default.
type Extension struct {
	catalog taskauthz.Catalog
}

// NewExtension builds the extension. The catalog is its only dependency.
func NewExtension(catalog taskauthz.Catalog) *Extension {
	return &Extension{catalog: catalog}
}

// rules is the per-task config carried in the extension properties:
// state -> command -> [logical principal names].
type rules map[string]map[string][]string

// Execute runs in the PRE_RESUME phase, before the task resumes; a non-nil
// return aborts the step.
func (e *Extension) Execute(ctx context.Context, record *store.TaskRecord, payload map[string]any, properties json.RawMessage) error {
	// Absent rules mean nothing is permitted here — deny (403) rather than
	// surfacing the unmarshal failure as a 500.
	if len(properties) == 0 {
		return fmt.Errorf("%w: no authorization rules configured for task %q", ErrForbidden, record.TaskID)
	}
	var r rules
	if err := json.Unmarshal(properties, &r); err != nil {
		return fmt.Errorf("task authz: invalid properties for task %q: %w", record.TaskID, err)
	}

	command, _ := payload["__command"].(string)

	// Deny-by-default: a (state, command) with no rule is not permitted here.
	allowed := r[record.State][command]
	if len(allowed) == 0 {
		return fmt.Errorf("%w: command %q is not permitted in state %q", ErrForbidden, command, record.State)
	}

	in, ok := taskauthz.InputFromContext(ctx)
	if !ok {
		return ErrUnauthenticated
	}

	switch in.Kind {
	case taskauthz.KindUser:
		return e.authorizeUser(ctx, record, in, allowed)
	case taskauthz.KindClient:
		return e.authorizeClient(in, allowed)
	default:
		return ErrUnauthenticated
	}
}

// authorizeUser allows the call iff the caller may act as some allowed name —
// they hold its token role and their company owns the task's root workflow in
// that role. Because a write is one yes/no decision, any single match suffices;
// the read path, which selects content rather than deciding, instead resolves a
// single acting role.
func (e *Extension) authorizeUser(ctx context.Context, record *store.TaskRecord, in taskauthz.Input, allowed []string) error {
	// Eligible resolves ownership only if the caller holds one of the allowed
	// roles, so the common denial below costs no database lookup.
	eligible, err := e.catalog.Eligible(ctx, in, record.RootWorkflowID, allowed)
	if err != nil {
		return fmt.Errorf("task authz: %w", err)
	}
	if !eligible.HoldsAny(allowed) {
		return fmt.Errorf("%w: caller holds no role allowed for this command", ErrForbidden)
	}
	// A user principal reaching here with no resolver means Layer 1 attached none
	// — a wiring bug. Say so rather than reporting it as "does not own".
	if in.OwnedRoles == nil {
		return fmt.Errorf("%w: ownership could not be resolved", ErrForbidden)
	}
	if eligible.Any(allowed) {
		return nil
	}
	return fmt.Errorf("%w: caller's company does not own this task in the required role", ErrForbidden)
}

// authorizeClient allows the call iff some allowed name is a catalog client whose
// mapped client id equals the caller's client id.
func (e *Extension) authorizeClient(in taskauthz.Input, allowed []string) error {
	for _, name := range allowed {
		if want, isClient := e.catalog.Clients[name]; isClient && want != "" && want == in.ClientID {
			return nil
		}
	}
	return fmt.Errorf("%w: client %q is not permitted for this command", ErrForbidden, in.ClientID)
}
