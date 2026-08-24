// Package taskauthz is the shared vocabulary of task authorization: who the
// caller is, and what the deployment's logical principal names mean.
//
// It holds no policy. Two packages build on it, and neither depends on the other:
//
//   - internal/tasks/authzgate — Layer 1. An HTTP middleware that resolves the
//     caller's identity into an Input and attaches it to the request context,
//     along with a lazy ownership resolver.
//   - internal/tasks/extensions/authz — Layer 2. A PRE_RESUME task extension
//     deciding whether the caller may run a command on a task.
//
// These types used to live in the extension. But only one of the two packages
// that need them is a core task extension — Layer 1 is HTTP middleware — so the
// shared half cannot live under extensions/ without the transport layer importing
// a write-policy package for types that have nothing to do with write policy.
package taskauthz

import "context"

// PrincipalKind distinguishes a portal user from an M2M service client.
type PrincipalKind string

const (
	KindUser   PrincipalKind = "user"
	KindClient PrincipalKind = "client"
)

// OwnedRolesFunc lazily resolves which logical owner roles the caller's company
// satisfies for the task's root workflow (keyed by the catalog's logical role
// names, e.g. "trader"/"cha"). It performs the database work, so callers invoke
// it only once they know the answer can matter. It is nil for client principals.
type OwnedRolesFunc func(ctx context.Context, rootWorkflowID string) (map[string]bool, error)

// Input is the authorization context Layer 1 resolves and attaches for a Layer-2
// evaluator to judge. It carries no domain detail — only the caller's kind, token
// roles, client id, and a lazy ownership resolver.
type Input struct {
	Kind       PrincipalKind
	Roles      []string
	ClientID   string
	OwnedRoles OwnedRolesFunc
}

type ctxKey struct{}

// WithInput returns a context carrying in for an evaluator to read. Layer 1 is
// its only intended caller: minting an Input elsewhere would let a handler assert
// an identity nothing verified.
func WithInput(ctx context.Context, in Input) context.Context {
	return context.WithValue(ctx, ctxKey{}, in)
}

// InputFromContext returns the Input attached by WithInput; ok is false when none
// is present (an unauthenticated request).
func InputFromContext(ctx context.Context) (Input, bool) {
	in, ok := ctx.Value(ctxKey{}).(Input)
	return in, ok
}
