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
