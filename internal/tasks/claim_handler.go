package tasks

import (
	"context"
	"errors"
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
	errClaimHeld         = "this task is claimed by another user; refetch it to see who"
	errClaimTaskComplete = "this task is completed and can no longer be claimed"
	errNotClaimHolder    = "this task is claimed by another user"
	errClaimRequired     = "claim this task before acting on it"
	errClaimsOff         = "task claims are not used in this deployment"
)

// Claim facts passed to the renderer with the caller's role claims, so a render
// config can show or hide a section by who holds the task's claim, e.g.
// "visibleWhen": {"requireClaim": "claim:mine"}. Exactly one is true on every read.
const (
	ClaimFactMine  = "claim:mine"  // the caller holds the claim
	ClaimFactOther = "claim:other" // another user holds it
	ClaimFactNone  = "claim:none"  // nobody holds it
)

// UserService resolves a user's id to the name shown for them, such as a claim holder's.
type UserService interface {
	// GetDisplayName returns the name to show for userID, or "" when there is none.
	GetDisplayName(ctx context.Context, userID string) (string, error)
}

// taskResponse is the task view served by GET /api/v1/tasks/{id}: core's zone view
// and who holds the task's claim, as seen by the caller.
type taskResponse struct {
	zoneview.ZoneView
	Claim taskClaim `json:"claim"`
}

// taskClaim is the task's claim as the caller sees it. It is always present, so a
// client can tell whether it must claim the task before acting on it.
type taskClaim struct {
	// Holder is who holds the claim; absent when the task is unclaimed.
	Holder    *claimHolder `json:"holder,omitempty"`
	ClaimedAt *time.Time   `json:"claimed_at,omitempty"`
	// Mine reports whether the caller holds the claim. It is decided here, from the
	// caller's internal user id, so clients never compare ids or emails themselves.
	Mine bool `json:"mine"`
	// RequiredToSubmit reports whether this deployment requires the caller to hold
	// the claim to submit a step (see taskauthz.Input.Claims).
	RequiredToSubmit bool `json:"required_to_submit"`
}

type claimHolder struct {
	ID string `json:"id"`
	// Name is the holder's name, or "" if it could not be found.
	Name string `json:"name"`
}

// claimOf describes record's claim as the caller (in) sees it.
func (h *HTTPHandler) claimOf(ctx context.Context, in taskauthz.Input, record store.TaskRecord) taskClaim {
	c := taskClaim{
		ClaimedAt:        record.ClaimedAt,
		Mine:             isClaimHolder(in, record),
		RequiredToSubmit: in.Kind == taskauthz.KindUser && in.Claims == taskauthz.ClaimsRequired,
	}
	if record.ClaimedBy == "" {
		return c
	}
	c.Holder = &claimHolder{ID: record.ClaimedBy}
	if h.Users != nil {
		// A missing name is not worth failing the read over: the view still says the
		// task is claimed, and by whom.
		name, err := h.Users.GetDisplayName(ctx, record.ClaimedBy)
		if err != nil {
			slog.WarnContext(ctx, "tasks: failed to resolve claim holder's name", "taskId", record.TaskID, "error", err)
		}
		c.Holder.Name = name
	}
	return c
}

// withClaimFacts returns the caller's role claims with the claim facts added, for
// the renderer.
func withClaimFacts(claims map[string]bool, in taskauthz.Input, record store.TaskRecord) map[string]bool {
	out := make(map[string]bool, len(claims)+3)
	for k, v := range claims {
		out[k] = v
	}
	mine := isClaimHolder(in, record)
	out[ClaimFactMine] = mine
	out[ClaimFactOther] = record.ClaimedBy != "" && !mine
	out[ClaimFactNone] = record.ClaimedBy == ""
	return out
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
// working on it. Claiming a task you already hold succeeds. Whether claims are used,
// and whether one is needed to act on the task, is the deployment's policy
// (taskauthz.Input.Claims); any user who can read a task may claim it.
//
//	POST /api/v1/tasks/{id}/claim
//
// 204 on success: the task view changes with the claim, so the caller refetches it.
// 404 where the deployment does not use claims. 409 when another user holds the claim
// or the task is completed.
func (h *HTTPHandler) HandleClaimTask(w http.ResponseWriter, r *http.Request) {
	h.handleClaim(w, r, "claim", h.Claims.ClaimTask)
}

// HandleReleaseTask releases the calling user's claim on a task. Releasing a task
// nobody holds succeeds.
//
//	POST /api/v1/tasks/{id}/release
//
// 204 on success. 403 when another user holds the claim. 404 where the deployment
// does not use claims.
func (h *HTTPHandler) HandleReleaseTask(w http.ResponseWriter, r *http.Request) {
	h.handleClaim(w, r, "release", h.Claims.ReleaseTask)
}

// handleClaim runs a claim or release (op names it in logs and the audit trail)
// for the calling user, once they are known to be allowed to read the task.
func (h *HTTPHandler) handleClaim(w http.ResponseWriter, r *http.Request, op string, apply func(ctx context.Context, taskID, holder string) error) {
	ctx := r.Context()
	taskID := r.PathValue("id")
	if taskID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}

	in, ok := taskauthz.InputFromContext(ctx)
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
		return
	}
	// A claim says which person is working a task. A client completes steps by
	// callback token and has no use for one.
	if in.Kind != taskauthz.KindUser {
		httputil.Error(w, r, http.StatusForbidden, errClaimUsersOnly)
		return
	}
	// Answered before the task is read, so it cannot be used to probe task ids.
	if in.Claims == taskauthz.ClaimsOff {
		httputil.Error(w, r, http.StatusNotFound, errClaimsOff)
		return
	}
	if in.UserID == "" {
		slog.ErrorContext(ctx, "tasks: user principal without an internal user id", "taskId", taskID)
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
		return
	}

	record, ok := h.Store.GetTask(ctx, taskID)
	if !ok {
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return
	}
	// Only someone who can read the task may claim or release it. A denial is answered
	// as not found, as HandleGetTask does, so it cannot be used to probe task ids.
	if _, err := readauthz.Resolve(ctx, h.AuthzCatalog, in, record.RenderConfig, record.CaseID); err != nil {
		if !errors.Is(err, readauthz.ErrDenied) {
			httputil.InternalServerError(w, r, "tasks: failed to resolve read access", err, "taskId", taskID)
			return
		}
		slog.WarnContext(ctx, "tasks: "+op+" denied: no read access", "taskId", taskID)
		h.auditClaim(ctx, taskID, op, "task read access denied")
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return
	}

	err := apply(ctx, taskID, in.UserID)
	switch {
	case err == nil:
		slog.InfoContext(ctx, "tasks: "+op+" applied", "taskId", taskID)
		h.auditClaim(ctx, taskID, op, "")
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, orchestrator.ErrTaskNotFound):
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
	case errors.Is(err, orchestrator.ErrClaimHeld):
		h.auditClaim(ctx, taskID, op, "task is claimed by another user")
		httputil.Error(w, r, http.StatusConflict, errClaimHeld)
	case errors.Is(err, orchestrator.ErrTaskCompleted):
		h.auditClaim(ctx, taskID, op, "task is completed")
		httputil.Error(w, r, http.StatusConflict, errClaimTaskComplete)
	case errors.Is(err, orchestrator.ErrNotClaimHolder):
		h.auditClaim(ctx, taskID, op, "task is claimed by another user")
		httputil.Error(w, r, http.StatusForbidden, errNotClaimHolder)
	case errors.Is(err, orchestrator.ErrHolderRequired):
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
	default:
		httputil.InternalServerError(w, r, "tasks: failed to "+op+" task", err, "taskId", taskID)
	}
}

// requireClaim enforces the deployment's claim policy on a step submission. When
// the caller is a user and the deployment requires a claim to submit
// (taskauthz.ClaimsRequired), they must hold the task's claim; otherwise
// it answers 403 (404 for a missing task) and returns false. It returns true when
// the submission may go ahead.
//
// Clients are never held to it: they complete steps by callback token and cannot
// hold a claim. Users are held to it on the callback route as well, because a
// callback token is not signed and a user who can see a task's step can build one.
//
// The check and the submission are separate calls, so a holder who releases the
// claim while their submission is in flight can still have it accepted. The
// workflow accepts only one submission per step, so the step is never completed
// twice. This is safe only because nobody but the holder can release a claim: a
// forced or supervisor release would let another user take the claim between the
// check and the submission, and would need the check fenced with the submission
// (see core taskflow's integration guide).
func (h *HTTPHandler) requireClaim(w http.ResponseWriter, r *http.Request, taskID string, logAttrs ...any) bool {
	ctx := r.Context()
	in, ok := taskauthz.InputFromContext(ctx)
	if !ok || in.Kind != taskauthz.KindUser || in.Claims != taskauthz.ClaimsRequired {
		return true
	}

	record, ok := h.Store.GetTask(ctx, taskID)
	switch {
	case !ok:
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return false
	case isClaimHolder(in, record):
		return true
	case record.ClaimedBy == "":
		slog.InfoContext(ctx, "tasks: submission refused: task not claimed", logAttrs...)
		httputil.Error(w, r, http.StatusForbidden, errClaimRequired)
		return false
	default:
		slog.InfoContext(ctx, "tasks: submission refused: task claimed by another user", logAttrs...)
		httputil.Error(w, r, http.StatusForbidden, errNotClaimHolder)
		return false
	}
}

// auditClaim records a claim or release of a task. failure is the reason it was
// refused, or "" when it was applied. The audit trail has no claim action of its
// own, so both are updates to the task, told apart by the operation.
func (h *HTTPHandler) auditClaim(ctx context.Context, taskID, op, failure string) {
	metadata := map[string]any{"operation": op}
	if failure != "" {
		metadata["error"] = failure
	}
	h.Audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventTask,
		Action:     nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetTask,
		TargetID:   taskID,
		Failure:    failure != "",
		Metadata:   metadata,
	})
}
