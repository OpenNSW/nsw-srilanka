// Package authz provides nsw-srilanka's task-step authorization extension: a
// PRE_RESUME policy gate that decides whether the caller may run a command on a
// task at its current state.
//
// It is a pure evaluator. The API layer resolves the caller's identity and
// attaches it — together with a lazy ownership resolver — to the request context
// (see Input); this extension only matches that against the per-task policy and
// the catalog. It resolves ownership only when a user rule actually needs it, and
// never touches domain services directly.
package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/OpenNSW/core/taskflow/store"
)

// Sentinel errors mapped to HTTP status by the task handler. errors.Is still
// matches after the orchestrator wraps them with %w; any other error is a 500.
var (
	ErrUnauthenticated = errors.New("task authz: unauthenticated")
	ErrForbidden       = errors.New("task authz: forbidden")
)

// PrincipalKind distinguishes a portal user from an M2M service client.
type PrincipalKind string

const (
	KindUser   PrincipalKind = "user"
	KindClient PrincipalKind = "client"
)

// OwnedRolesFunc lazily resolves which logical owner roles the caller satisfies
// for the task's root workflow (keyed by the catalog's logical user names, e.g.
// "trader"/"cha"). It performs the DB work, so the extension calls it only when a
// user rule actually needs it. It is nil for client principals.
type OwnedRolesFunc func(ctx context.Context, rootWorkflowID string) (map[string]bool, error)

// Input is the authorization context the API layer resolves and attaches for the
// extension to evaluate. It carries no domain detail — only the caller's kind,
// token roles, client id, and a lazy ownership resolver.
type Input struct {
	Kind       PrincipalKind
	Roles      []string
	ClientID   string
	OwnedRoles OwnedRolesFunc
}

type ctxKey struct{}

// WithInput returns a context carrying in for the extension to read.
func WithInput(ctx context.Context, in Input) context.Context {
	return context.WithValue(ctx, ctxKey{}, in)
}

// InputFromContext returns the Input attached by WithInput; ok is false when none
// is present (an unauthenticated request).
func InputFromContext(ctx context.Context) (Input, bool) {
	in, ok := ctx.Value(ctxKey{}).(Input)
	return in, ok
}

// Catalog is the global principal catalog (configs/task_authz.json). Per-task
// rules reference the logical names resolved here: users to a token role,
// clients to an OAuth2 client id.
type Catalog struct {
	Users   map[string]string `json:"users"`   // logical name -> token role
	Clients map[string]string `json:"clients"` // logical name -> client id
}

// Validate reports whether the catalog is internally consistent.
func (c *Catalog) Validate() error {
	if c == nil {
		return errors.New("task authz: nil catalog")
	}
	if len(c.Users) == 0 && len(c.Clients) == 0 {
		return errors.New("task authz: catalog defines no users or clients")
	}
	for name, role := range c.Users {
		if role == "" {
			return fmt.Errorf("task authz: user %q is missing a token role", name)
		}
	}
	for name, clientID := range c.Clients {
		if clientID == "" {
			return fmt.Errorf("task authz: client %q is missing a client id", name)
		}
	}
	return nil
}

// LoadCatalog reads and validates the catalog file at path.
func LoadCatalog(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("task authz: read catalog %s: %w", path, err)
	}
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("task authz: parse catalog %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Extension is the PRE_RESUME task extension enforcing, per task state and
// command, which principals may advance a task: users by token role + resolved
// ownership, M2M clients by client id. It is deny-by-default.
type Extension struct {
	catalog *Catalog
}

// NewExtension builds the extension. The catalog is its only dependency.
func NewExtension(catalog *Catalog) *Extension {
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

	in, ok := InputFromContext(ctx)
	if !ok {
		return ErrUnauthenticated
	}

	switch in.Kind {
	case KindUser:
		return e.authorizeUser(ctx, record, in, allowed)
	case KindClient:
		return e.authorizeClient(in, allowed)
	default:
		return ErrUnauthenticated
	}
}

// authorizeUser allows the call iff some allowed name is a catalog user whose
// token role the caller holds and whose company owns the task's root workflow in
// that role. Ownership is resolved lazily and only after a role first matches, so
// a caller whose roles match no allowed user is denied with no ownership lookup.
func (e *Extension) authorizeUser(ctx context.Context, record *store.TaskRecord, in Input, allowed []string) error {
	held := roleSet(in.Roles)

	var candidates []string
	for _, name := range allowed {
		if role, isUser := e.catalog.Users[name]; isUser && held[role] {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("%w: caller holds no role allowed for this command", ErrForbidden)
	}
	if in.OwnedRoles == nil {
		return fmt.Errorf("%w: ownership could not be resolved", ErrForbidden)
	}

	owned, err := in.OwnedRoles(ctx, record.RootWorkflowID)
	if err != nil {
		return fmt.Errorf("task authz: resolve ownership: %w", err)
	}
	for _, name := range candidates {
		if owned[name] {
			return nil
		}
	}
	return fmt.Errorf("%w: caller's company does not own this task in the required role", ErrForbidden)
}

// authorizeClient allows the call iff some allowed name is a catalog client whose
// mapped client id equals the caller's client id.
func (e *Extension) authorizeClient(in Input, allowed []string) error {
	for _, name := range allowed {
		if want, isClient := e.catalog.Clients[name]; isClient && want != "" && want == in.ClientID {
			return nil
		}
	}
	return fmt.Errorf("%w: client %q is not permitted for this command", ErrForbidden, in.ClientID)
}

func roleSet(roles []string) map[string]bool {
	m := make(map[string]bool, len(roles))
	for _, r := range roles {
		m[r] = true
	}
	return m
}
