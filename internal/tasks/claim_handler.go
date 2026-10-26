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

// claimHolder is who holds a task's claim. It carries no user id: the server decides
// whether the caller is the holder (taskClaim.Mine), so clients never need one.
type claimHolder struct {
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
// working on it. Claiming a task you already hold succeeds. Whether claims are used is
// the deployment's policy (taskauthz.Input.Claims). Any user who can read the task may
// claim it.
//
//	POST /api/v1/tasks/{id}/claim
//
// 204 on success: the task view changes with the claim, so the caller refetches it.
// 404 where the deployment does not use claims. 409 when another user holds the
// claim or the task is completed.
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

// handleClaim runs a claim or release (op names it in logs and the audit trail) for
// the calling user, once they are known to be allowed to read the task.
func (h *HTTPHandler) handleClaim(w http.ResponseWriter, r *http.Request, op string, apply func(ctx context.Context, taskID, holder string) error) {
	ctx := r.Context()
	taskID := r.PathValue("id")
	if taskID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}
	in, ok := h.claimant(w, r, taskID)
	if !ok || !h.admitClaimant(w, r, op, taskID, in) {
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
	if in.Claims == taskauthz.ClaimsOff {
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

// admitClaimant reports whether in may claim or release the task, answering when
// not: they must be able to read it.
func (h *HTTPHandler) admitClaimant(w http.ResponseWriter, r *http.Request, op, taskID string, in taskauthz.Input) bool {
	ctx := r.Context()
	record, ok := h.Store.GetTask(ctx, taskID)
	if !ok {
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return false
	}
	// A read denial is answered as not found, as HandleGetTask does, so it cannot be
	// used to probe task ids.
	canRead, err := h.readable(ctx, in, record)
	if err != nil {
		httputil.InternalServerError(w, r, "tasks: failed to resolve read access", err, "taskId", taskID)
		return false
	}
	if !canRead {
		slog.WarnContext(ctx, "tasks: "+op+" denied: no read access", "taskId", taskID)
		h.auditClaim(ctx, taskID, op, "task read access denied")
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return false
	}
	return true
}

// readable reports whether in may read record. A denial is (false, nil); any other
// failure is an error, not a denial.
func (h *HTTPHandler) readable(ctx context.Context, in taskauthz.Input, record store.TaskRecord) (bool, error) {
	_, err := readauthz.Resolve(ctx, h.AuthzCatalog, in, record.RenderConfig, record.CaseID)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, readauthz.ErrDenied):
		return false, nil
	default:
		return false, err
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
	h.auditEvent(ctx, nswaudit.Event{
		EventType:  nswaudit.EventTask,
		Action:     nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetTask,
		TargetID:   taskID,
		Failure:    failure != "",
		Metadata:   metadata,
	})
}
