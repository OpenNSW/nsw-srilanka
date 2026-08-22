// Package readauthz decides whether a caller may read a task.
//
// It is the read-side counterpart of the task-step authorization extension, and
// exists as its own package rather than a task extension because core has no read
// hook — extensions only fire on CompleteTaskStep. Like the write-side evaluator
// it is pure: Layer 1 (internal/tasks/authzgate) resolves the caller's identity
// and a lazy ownership resolver into a taskauthz.Input, and this package matches
// that against the catalog. It touches no domain service and reads no
// configuration file.
//
// Eligibility — holding a role and owning the task's consignment in it — is
// decided by taskauthz, shared with the write path. Reading a task requires being
// eligible for at least one role on its consignment; which role that is does not
// change the answer, only who is allowed to ask.
package readauthz

import (
	"context"
	"errors"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// ErrDenied means the caller may not read this task. Handlers must translate it
// to a 404 (not a 403), so a denied read is indistinguishable from a task that
// does not exist and cannot be used to probe which task ids are real.
var ErrDenied = errors.New("task read authz: denied")

// Evaluator decides read access against the global catalog.
type Evaluator struct {
	catalog taskauthz.Catalog
	roles   []string // every logical role name in the catalog, resolved once
}

// NewEvaluator builds an Evaluator. The catalog must define at least one role;
// with none, nobody could ever be eligible and every read would be denied.
func NewEvaluator(cat taskauthz.Catalog) (*Evaluator, error) {
	if len(cat.Roles) == 0 {
		return nil, errors.New("readauthz: catalog defines no roles")
	}
	roles := make([]string, 0, len(cat.Roles))
	for name := range cat.Roles {
		roles = append(roles, name)
	}
	return &Evaluator{catalog: cat, roles: roles}, nil
}

// Authorize reports whether the caller may read the task rooted at
// rootWorkflowID, which is the id of the consignment the task belongs to. It
// returns ErrDenied when they may not; any other error is a real failure, not a
// denial, and must not be answered as one.
func (e *Evaluator) Authorize(ctx context.Context, in taskauthz.Input, rootWorkflowID string) error {
	eligible, err := e.catalog.Eligible(ctx, in, rootWorkflowID, e.roles)
	if err != nil {
		return err
	}
	if !eligible.Any(e.roles) {
		return ErrDenied
	}
	return nil
}
