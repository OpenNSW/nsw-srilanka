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
//
// The package holds no state: the catalog is passed per call, so there is nothing
// to construct and no boot-time failure path.
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

// Authorize reports whether the caller may read the task rooted at
// rootWorkflowID, which is the id of the consignment the task belongs to. It
// returns ErrDenied when they may not; any other error is a real failure, not a
// denial, and must not be answered as one.
//
// A catalog defining no roles denies every caller. That is the fail-closed
// answer, and it cannot arise in the wiring anyway: the read route is served
// through the authz gate, whose construction already requires the catalog to
// define both owner roles.
func Authorize(ctx context.Context, cat taskauthz.Catalog, in taskauthz.Input, rootWorkflowID string) error {
	// allowed is every role the catalog defines: a read is admitted by owning the
	// task's consignment in any one of them.
	allowed := cat.RoleNames()
	eligible, err := cat.Eligible(ctx, in, rootWorkflowID, allowed)
	if err != nil {
		return err
	}
	if !eligible.Any(allowed) {
		return ErrDenied
	}
	return nil
}
