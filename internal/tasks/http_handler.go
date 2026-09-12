// Package tasks hosts the HTTP surface for the core-based task orchestrator
// (the core/taskflow port of the old internal/taskv2 HTTP handler).
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/readauthz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

const (
	errTaskIDRequired      = "task id is required"
	errTaskNotFound        = "task not found"
	errAuthenticationReq   = "authentication required"
	errForbiddenTaskAction = "you may not perform this action on this task"
	errInvalidRequestBody  = "invalid request body"
	errRequestBodyTooLarge = "request body too large"
)

// TaskFetcher is the narrow surface HandleGetTask needs from the task store.
type TaskFetcher interface {
	GetTask(ctx context.Context, taskID string) (store.TaskRecord, bool)
}

type HTTPHandler struct {
	Manager   *orchestrator.TaskManager
	Store     TaskFetcher
	Assembler *zoneview.ZoneViewAssembler
	// AuthzCatalog names the logical roles a reader may own the task's
	// consignment in. HandleGetTask authorizes against it.
	AuthzCatalog    taskauthz.Catalog
	MaxRequestBytes int64
}

func NewHTTPHandler(
	manager *orchestrator.TaskManager,
	store TaskFetcher,
	assembler *zoneview.ZoneViewAssembler,
	authzCatalog taskauthz.Catalog,
	maxRequestBytes int64,
) *HTTPHandler {
	return &HTTPHandler{
		Manager:         manager,
		Store:           store,
		Assembler:       assembler,
		AuthzCatalog:    authzCatalog,
		MaxRequestBytes: maxRequestBytes,
	}
}

// HandleGetTask returns the ZoneView payload for a single task, scoped to the
// caller: they must own the task's consignment in a role the task's render
// config admits, and the claims resolved for them decide which sections of the
// view they see.
//
//	GET /api/v1/tasks/{id}
func (h *HTTPHandler) HandleGetTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	taskID := r.PathValue("id")
	if taskID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}

	// Attached by the task authz gate. Absent means no usable principal, which
	// the scope middleware should already have rejected.
	in, ok := taskauthz.InputFromContext(ctx)
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
		return
	}

	record, ok := h.Store.GetTask(ctx, taskID)
	if !ok {
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return
	}

	// RootWorkflowID is the consignment id, so this decides the caller's access
	// from their role-tied ownership of the task's consignment, and returns the
	// claims that shape their view of it.
	claims, err := readauthz.Resolve(ctx, h.AuthzCatalog, in, record.RenderConfig, record.RootWorkflowID)
	if err != nil {
		if !errors.Is(err, readauthz.ErrDenied) {
			httputil.InternalServerError(w, r, "tasks: failed to resolve read access", err, "taskId", taskID)
			return
		}
		// Answer with the not-found status and text, so a denied read is
		// indistinguishable from a task that does not exist and cannot be used to
		// probe which task ids are real. Mirrors GET /api/v1/consignments/{id}.
		slog.WarnContext(ctx, "tasks: read authorization denied", "taskId", taskID)
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return
	}

	zv, err := h.Assembler.Assemble(ctx, record, claims)
	if err != nil {
		httputil.InternalServerError(w, r, "tasks: failed to assemble zone view", err, "taskId", taskID)
		return
	}

	httputil.JSON(w, http.StatusOK, zv)
}

// HandleCompleteTaskStep advances a task by submitting a step payload.
//
//	POST /api/v1/tasks/{id}
func (h *HTTPHandler) HandleCompleteTaskStep(w http.ResponseWriter, r *http.Request) {
	// TODO: retrieve the authenticated context and validate it against the
	// task's ownership bounds before completing the step.
	taskID := r.PathValue("id")
	if taskID == "" {
		slog.ErrorContext(r.Context(), "tasks: missing task id in request")
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	fail := func(status int, message string, err error) {
		slog.ErrorContext(r.Context(), "tasks: failed to parse request", "taskId", taskID, "error", err)
		httputil.Error(w, r, status, message)
	}

	// The body must contain at most one JSON value: json.Decoder.Decode only parses the
	// first value and silently ignores anything after it, so a second Decode call is
	// required to confirm nothing trails it.
	var req completeTaskStepRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fail(http.StatusRequestEntityTooLarge, errRequestBodyTooLarge, err)
			return
		}

		// An empty body is tolerated here and caught by the command-required check below;
		// only fail on genuinely malformed JSON.
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyReadAfterClose) {
			fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: malformed JSON"))
			return
		}

		// If unexpected data follows the first JSON value, reject the request.
	} else if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fail(http.StatusRequestEntityTooLarge, errRequestBodyTooLarge, err)
			return
		}
		fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: unexpected data after JSON value"))
		return
	}

	if req.Command == "" {
		fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: must contain 'command' (string)"))
		return
	}

	payload := req.Payload

	// Validate system metadata collision
	if payload != nil {
		if _, exists := payload["__command"]; exists {
			fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request payload: '__command' is a reserved system key"))
			return
		}
	}

	if payload == nil {
		payload = make(map[string]any)
	}

	command := req.Command
	payload["__command"] = command

	slog.InfoContext(r.Context(), "tasks: processing complete step command", "taskId", taskID, "command", command)

	if err := h.Manager.CompleteTaskStep(r.Context(), taskID, payload); err != nil {
		switch {
		case errors.Is(err, taskauthzext.ErrUnauthenticated):
			httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
		case errors.Is(err, taskauthzext.ErrForbidden):
			slog.WarnContext(r.Context(), "tasks: authorization denied", "taskId", taskID, "command", command, "error", err)
			httputil.Error(w, r, http.StatusForbidden, errForbiddenTaskAction)
		default:
			httputil.InternalServerError(w, r, "tasks: failed to complete task step", err, "taskId", taskID)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// completeTaskStepRequest is the JSON envelope HandleCompleteTaskStep accepts:
// {"command": "...", "payload": {...}}. Payload stays map[string]any because its
// contents are genuinely dynamic per task type; only the envelope around it has
// a fixed shape.
type completeTaskStepRequest struct {
	Command string         `json:"command"`
	Payload map[string]any `json:"payload"`
}
