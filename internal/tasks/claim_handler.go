package tasks

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/taskflow/orchestrator"
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
)

// TaskClaimer is the narrow surface the claim routes need from the task manager.
// *orchestrator.TaskManager satisfies it.
type TaskClaimer interface {
	ClaimTask(ctx context.Context, taskID, holder string) error
	ReleaseTask(ctx context.Context, taskID, holder string) error
}

// HandleClaimTask claims a task for the calling user, so others can see who is
// working on it. Claiming a task you already hold succeeds. Whether a claim is needed
// to act on the task is the deployment's policy (taskauthz.Input.RequireClaimToSubmit);
// any user who can read a task may claim it.
//
//	POST /api/v1/tasks/{id}/claim
//
// 204 on success: the task view changes with the claim, so the caller refetches it.
// 409 when another user holds the claim or the task is completed.
func (h *HTTPHandler) HandleClaimTask(w http.ResponseWriter, r *http.Request) {
	h.handleClaim(w, r, "claim", h.Claims.ClaimTask)
}

// HandleReleaseTask releases the calling user's claim on a task. Releasing a task
// nobody holds succeeds.
//
//	POST /api/v1/tasks/{id}/release
//
// 204 on success. 403 when another user holds the claim.
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
	if _, err := readauthz.Resolve(ctx, h.AuthzCatalog, in, record.RenderConfig, record.RootWorkflowID); err != nil {
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
// (taskauthz.Input.RequireClaimToSubmit), they must hold the task's claim; otherwise
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
// twice.
func (h *HTTPHandler) requireClaim(w http.ResponseWriter, r *http.Request, taskID string, logAttrs ...any) bool {
	ctx := r.Context()
	in, ok := taskauthz.InputFromContext(ctx)
	if !ok || in.Kind != taskauthz.KindUser || !in.RequireClaimToSubmit {
		return true
	}

	record, ok := h.Store.GetTask(ctx, taskID)
	switch {
	case !ok:
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return false
	case in.UserID != "" && record.ClaimedBy == in.UserID:
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
