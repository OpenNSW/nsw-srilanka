package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/readauthz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

const (
	errClaimUsersOnly    = "only users can claim tasks"
	errClaimChanged      = "this task's claim just changed; refetch the task and try again"
	errClaimTaskComplete = "this task is completed and can no longer be claimed"
	errNotClaimHolder    = "this task is claimed by another user"
	errClaimsOff         = "task claims are not used in this deployment"
)

// UserService resolves a user's id to the name shown for them, such as a claim holder's.
type UserService interface {
	// GetDisplayName returns the name to show for userID, or "" when there is none.
	GetDisplayName(ctx context.Context, userID string) (string, error)
}

// taskResponse is the task view served by GET /api/v1/tasks/{id}: core's zone view
// and, where claims are on, who holds the task's claim, as seen by the caller.
type taskResponse struct {
	zoneview.ZoneView
	Claim *taskClaim `json:"claim,omitempty"`
}

// taskClaim is the task's claim as the caller sees it. The task view carries it only
// where claims are on.
type taskClaim struct {
	// Holder is who holds the claim; absent when the task is unclaimed.
	Holder    *claimHolder `json:"holder,omitempty"`
	ClaimedAt *time.Time   `json:"claimed_at,omitempty"`
	// Mine reports whether the caller holds the claim. It is decided here, from the
	// caller's internal user id, so clients never compare ids or emails themselves.
	Mine bool `json:"mine"`
}

// claimHolder is who holds a task's claim. It carries no user id: the server decides
// whether the caller is the holder (taskClaim.Mine), so clients never need one.
type claimHolder struct {
	// Name is the holder's name, or "" if it could not be found.
	Name string `json:"name"`
}

// claimOf describes record's claim as the caller (in) sees it.
func (h *HTTPHandler) claimOf(ctx context.Context, in taskauthz.Input, record store.TaskRecord) taskClaim {
	c := taskClaim{
		ClaimedAt: record.ClaimedAt,
		Mine:      isClaimHolder(in, record),
	}
	if record.ClaimedBy == "" {
		return c
	}
	c.Holder = &claimHolder{}
	if h.Users != nil {
		// A missing name is not worth failing the read over: the view still says the
		// task is claimed.
		name, err := h.Users.GetDisplayName(ctx, record.ClaimedBy)
		if err != nil {
			slog.WarnContext(ctx, "tasks: failed to resolve claim holder's name", "taskId", record.TaskID, "error", err)
		}
		c.Holder.Name = name
	}
	return c
}

// isClaimHolder reports whether the caller holds record's claim. A caller without a
// user id never does, so an unclaimed task's empty holder cannot match them.
func isClaimHolder(in taskauthz.Input, record store.TaskRecord) bool {
	return in.Kind == taskauthz.KindUser && in.UserID != "" && record.ClaimedBy == in.UserID
}

// TaskClaimer is the narrow surface the claim routes need from the task manager.
// *orchestrator.TaskManager satisfies it.
type TaskClaimer interface {
	ClaimTask(ctx context.Context, taskID, holder string) error
	ReleaseTask(ctx context.Context, taskID, holder string) error
}

// HandleClaimTask claims a task for the calling user, so others can see who is
// working on it. A claim is a hint, not a lock: submitting a step never checks it.
// Whether claims are used is the deployment's choice (HTTPHandler.ClaimsEnabled).
// Any user who can read the task may claim it. If another user holds the claim, the
// caller takes it over, for example when the holder is away. Claiming a task you
// already hold succeeds.
//
//	POST /api/v1/tasks/{id}/claim
//
// 204 on success: the task view changes with the claim, so the caller refetches it.
// 404 where the deployment does not use claims. 409 when the task is completed, or
// when the claim changed while it was being taken over.
func (h *HTTPHandler) HandleClaimTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	taskID, in, record, ok := h.claimTarget(w, r, "claim")
	if !ok {
		return
	}
	// A completed task keeps its claim as the record of who worked it, so it is
	// refused before a takeover could release that claim.
	if record.State == store.StateCompleted {
		h.answerClaim(w, r, "claim", taskID, nil, orchestrator.ErrTaskCompleted)
		return
	}

	op := "claim"
	var tk *takeover
	if record.ClaimedBy != "" && record.ClaimedBy != in.UserID {
		op, tk = "takeover", &takeover{previous: record.ClaimedBy}
		// Core has no takeover of its own, so release the holder's claim, then claim.
		// Should the claim then fail (the task completes, or a third user claims it
		// in between), the previous claim stays released: an accepted cost of keeping
		// claims in core simple. The audit trail says so (takeover.released).
		if err := h.Claims.ReleaseTask(ctx, taskID, tk.previous); err != nil {
			// Another user took the claim since the task was read.
			if errors.Is(err, orchestrator.ErrNotClaimHolder) {
				err = orchestrator.ErrClaimHeld
			}
			h.answerClaim(w, r, op, taskID, tk, err)
			return
		}
		tk.released = true
	}
	err := h.Claims.ClaimTask(ctx, taskID, in.UserID)
	if err != nil && tk != nil {
		slog.WarnContext(ctx, "tasks: takeover released the previous claim but could not claim the task", "taskId", taskID, "error", err)
	}
	h.answerClaim(w, r, op, taskID, tk, err)
}

// takeover is a takeover of another user's claim, for the audit trail: whose claim
// it was, and whether it was released before the takeover succeeded or failed.
type takeover struct {
	previous string
	released bool
}

// HandleReleaseTask releases the calling user's claim on a task. Releasing a task
// nobody holds succeeds. Nobody releases another user's claim: they take it over.
//
//	POST /api/v1/tasks/{id}/release
//
// 204 on success. 403 when another user holds the claim. 404 where the deployment
// does not use claims.
func (h *HTTPHandler) HandleReleaseTask(w http.ResponseWriter, r *http.Request) {
	taskID, in, _, ok := h.claimTarget(w, r, "release")
	if !ok {
		return
	}
	h.answerClaim(w, r, "release", taskID, nil, h.Claims.ReleaseTask(r.Context(), taskID, in.UserID))
}

// claimTarget returns the task a claim or release (op) is for, the calling user and
// the task's record, or answers and reports false when the caller may not claim or
// release it.
func (h *HTTPHandler) claimTarget(w http.ResponseWriter, r *http.Request, op string) (string, taskauthz.Input, store.TaskRecord, bool) {
	taskID := r.PathValue("id")
	if taskID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return "", taskauthz.Input{}, store.TaskRecord{}, false
	}
	in, ok := h.claimant(w, r, taskID)
	if !ok {
		return "", taskauthz.Input{}, store.TaskRecord{}, false
	}
	record, ok := h.admitClaimant(w, r, op, taskID, in)
	return taskID, in, record, ok
}

// answerClaim answers err, the outcome of a claim, takeover or release (op) of the
// task, and audits it. tk is the takeover, or nil for a plain claim or a release.
func (h *HTTPHandler) answerClaim(w http.ResponseWriter, r *http.Request, op, taskID string, tk *takeover, err error) {
	ctx := r.Context()
	switch {
	case err == nil:
		slog.InfoContext(ctx, "tasks: "+op+" applied", "taskId", taskID)
		h.auditClaim(ctx, taskID, op, tk, "")
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, orchestrator.ErrTaskNotFound):
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
	case errors.Is(err, orchestrator.ErrClaimHeld):
		h.auditClaim(ctx, taskID, op, tk, "task claim changed")
		httputil.Error(w, r, http.StatusConflict, errClaimChanged)
	case errors.Is(err, orchestrator.ErrTaskCompleted):
		h.auditClaim(ctx, taskID, op, tk, "task is completed")
		httputil.Error(w, r, http.StatusConflict, errClaimTaskComplete)
	case errors.Is(err, orchestrator.ErrNotClaimHolder):
		h.auditClaim(ctx, taskID, op, tk, "task is claimed by another user")
		httputil.Error(w, r, http.StatusForbidden, errNotClaimHolder)
	case errors.Is(err, orchestrator.ErrHolderRequired):
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
	default:
		httputil.InternalServerError(w, r, "tasks: failed to "+op+" task", err, "taskId", taskID)
	}
}

// claimant returns the calling user, or answers and reports false when the caller
// cannot claim or release a task at all: not authenticated, not a user, or in a
// deployment that does not use claims.
func (h *HTTPHandler) claimant(w http.ResponseWriter, r *http.Request, taskID string) (taskauthz.Input, bool) {
	ctx := r.Context()
	in, ok := taskauthz.InputFromContext(ctx)
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
		return in, false
	}
	// A claim says which person is working a task. A client completes steps by
	// callback token and has no use for one.
	if in.Kind != taskauthz.KindUser {
		httputil.Error(w, r, http.StatusForbidden, errClaimUsersOnly)
		return in, false
	}
	// Answered before the task is read, so it cannot be used to probe task ids.
	if !h.ClaimsEnabled {
		httputil.Error(w, r, http.StatusNotFound, errClaimsOff)
		return in, false
	}
	if in.UserID == "" {
		slog.ErrorContext(ctx, "tasks: user principal without an internal user id", "taskId", taskID)
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
		return in, false
	}
	return in, true
}

// admitClaimant returns the task's record if in may claim or release it, answering
// when not: they must be able to read it.
func (h *HTTPHandler) admitClaimant(w http.ResponseWriter, r *http.Request, op, taskID string, in taskauthz.Input) (store.TaskRecord, bool) {
	ctx := r.Context()
	record, ok := h.Store.GetTask(ctx, taskID)
	if !ok {
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return record, false
	}
	// A read denial is answered as not found, as HandleGetTask does, so it cannot be
	// used to probe task ids.
	canRead, err := h.readable(ctx, in, record)
	if err != nil {
		httputil.InternalServerError(w, r, "tasks: failed to resolve read access", err, "taskId", taskID)
		return record, false
	}
	if !canRead {
		slog.WarnContext(ctx, "tasks: "+op+" denied: no read access", "taskId", taskID)
		h.auditClaim(ctx, taskID, op, nil, "task read access denied")
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return record, false
	}
	return record, true
}

// readable reports whether in may read record. A denial is (false, nil); any other
// failure is an error, not a denial.
func (h *HTTPHandler) readable(ctx context.Context, in taskauthz.Input, record store.TaskRecord) (bool, error) {
	_, err := h.resolveRead(ctx, in, record)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, readauthz.ErrDenied):
		return false, nil
	default:
		return false, err
	}
}

// resolveRead is readauthz.Resolve for record, against its render config's read
// policy.
func (h *HTTPHandler) resolveRead(ctx context.Context, in taskauthz.Input, record store.TaskRecord) (map[string]bool, error) {
	if h.RenderConfigOf == nil {
		return nil, errors.New("tasks: no render config resolver configured")
	}
	renderConfig, err := h.RenderConfigOf(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	return readauthz.Resolve(ctx, h.AuthzCatalog, in, renderConfig, record.CaseID)
}

// auditClaim records a claim, takeover or release of a task. tk is the takeover, or
// nil. A takeover records whose claim it was (previous_holder) and whether that claim
// was released (previous_claim_released), which stays true when the takeover itself
// fails after the release. failure is the reason it was refused, or "" when it was
// applied. The audit trail has no claim action of its own, so all are updates to the
// task, told apart by the operation.
func (h *HTTPHandler) auditClaim(ctx context.Context, taskID, op string, tk *takeover, failure string) {
	metadata := map[string]any{"operation": op}
	if tk != nil {
		metadata["previous_holder"] = tk.previous
		metadata["previous_claim_released"] = tk.released
	}
	if failure != "" {
		metadata["error"] = failure
	}
	h.auditEvent(ctx, nswaudit.TaskEvent(nswaudit.ActionUpdate, taskID, failure != "", metadata))
}
