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
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/readauthz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

const (
	errTaskIDRequired       = "task id is required"
	errStepIDRequired       = "step id is required"
	errInvalidCallbackToken = "invalid callback token"
	errTaskNotFound         = "task not found"
	errAuthenticationReq    = "authentication required"
	errForbiddenTaskAction  = "you may not perform this action on this task"
	errInvalidRequestBody   = "invalid request body"
	errStaleStep            = "this step is no longer active; refetch the task"
	errRequestBodyTooLarge  = "request body too large"
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
	Audit           *nswaudit.Recorder
	MaxRequestBytes int64
}

func NewHTTPHandler(
	manager *orchestrator.TaskManager,
	store TaskFetcher,
	assembler *zoneview.ZoneViewAssembler,
	authzCatalog taskauthz.Catalog,
	audit *nswaudit.Recorder,
	maxRequestBytes int64,
) *HTTPHandler {
	return &HTTPHandler{
		Manager:         manager,
		Store:           store,
		Assembler:       assembler,
		AuthzCatalog:    authzCatalog,
		Audit:           audit,
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
		h.Audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventTask,
			Action:     nswaudit.ActionRead,
			TargetType: nswaudit.TargetTask,
			TargetID:   taskID,
			Failure:    true,
			Metadata:   map[string]any{"error": "task read access denied"},
		})
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

// HandleCompleteTaskStep advances a task by completing one of its steps. It is the
// portal's route.
//
//	POST /api/v1/tasks/{id}/steps/{stepId}
//
// {stepId} is the step being completed: the step_id the task view reported. Core
// completes only that step, so a submission for a step the task has since left is
// rejected (409) instead of applied to a later one.
func (h *HTTPHandler) HandleCompleteTaskStep(w http.ResponseWriter, r *http.Request) {
	// TODO: retrieve the authenticated context and validate it against the
	// task's ownership bounds before completing the step.
	taskID := r.PathValue("id")
	if taskID == "" {
		slog.ErrorContext(r.Context(), "tasks: missing task id in request")
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}
	stepID := r.PathValue("stepId")
	if stepID == "" {
		slog.ErrorContext(r.Context(), "tasks: missing step id in request", "taskId", taskID)
		httputil.Error(w, r, http.StatusBadRequest, errStepIDRequired)
		return
	}

	payload, ok := h.decodeSubmission(w, r, "taskId", taskID, "stepId", stepID)
	if !ok {
		return
	}
	// Read before the call: core may strip system keys from the payload.
	command := payload["__command"]
	err := h.Manager.CompleteTaskStep(r.Context(), taskID, stepID, payload)
	h.writeCompletion(w, r, err, "taskId", taskID, "stepId", stepID, "command", command)
}

// HandleCompleteTaskStepByToken advances a task by completing the step an external
// system was dispatched for. It is the route external reviewers call back on.
//
//	POST /api/v1/callbacks/{token}
//
// {token} is the opaque callbackToken the dispatch carried (core's
// plugins.CallbackToken). It names one step of one task, so a callback that arrives
// after the task has moved on is rejected (409) instead of completing a later step.
// The body is the same {command, payload} envelope HandleCompleteTaskStep takes.
func (h *HTTPHandler) HandleCompleteTaskStepByToken(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	// Decoded here only to reject a malformed token early and to log the task and
	// step it names; core decodes it again to complete the step.
	taskID, stepID, err := callbacktoken.Decode(token)
	if err != nil {
		slog.WarnContext(r.Context(), "tasks: invalid callback token", "error", err)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidCallbackToken)
		return
	}

	payload, ok := h.decodeSubmission(w, r, "taskId", taskID, "stepId", stepID)
	if !ok {
		return
	}
	// Read before the call: core may strip system keys from the payload.
	command := payload["__command"]
	err = h.Manager.CompleteTaskStepByToken(r.Context(), token, payload)
	h.writeCompletion(w, r, err, "taskId", taskID, "stepId", stepID, "command", command)
}

// decodeSubmission reads the {command, payload} envelope both completion routes take
// and returns the payload with the command set under the reserved "__command" key.
// On a bad request it writes the error response and returns false. logAttrs name
// the step in the log lines.
func (h *HTTPHandler) decodeSubmission(w http.ResponseWriter, r *http.Request, logAttrs ...any) (map[string]any, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	fail := func(status int, message string, err error) {
		slog.ErrorContext(r.Context(), "tasks: failed to parse request", append(logAttrs, "error", err)...)
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
			return nil, false
		}

		// An empty body is tolerated here and caught by the command-required check below;
		// only fail on genuinely malformed JSON.
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyReadAfterClose) {
			fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: malformed JSON"))
			return nil, false
		}

		// If unexpected data follows the first JSON value, reject the request.
	} else if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fail(http.StatusRequestEntityTooLarge, errRequestBodyTooLarge, err)
			return nil, false
		}
		fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: unexpected data after JSON value"))
		return nil, false
	}

	if req.Command == "" {
		fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: must contain 'command' (string)"))
		return nil, false
	}

	payload := req.Payload

	// Validate system metadata collision
	if payload != nil {
		if _, exists := payload["__command"]; exists {
			fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request payload: '__command' is a reserved system key"))
			return nil, false
		}
	}

	if payload == nil {
		payload = make(map[string]any)
	}

	payload["__command"] = req.Command

	slog.InfoContext(r.Context(), "tasks: processing complete step command", append(logAttrs, "command", req.Command)...)
	return payload, true
}

// writeCompletion answers a completion attempt: 204 when the workflow accepted the
// step, otherwise the error mapped to its status. logAttrs name the step.
func (h *HTTPHandler) writeCompletion(w http.ResponseWriter, r *http.Request, err error, logAttrs ...any) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, orchestrator.ErrStaleStep):
		slog.InfoContext(r.Context(), "tasks: stale step", append(logAttrs, "error", err)...)
		httputil.Error(w, r, http.StatusConflict, errStaleStep)
	case errors.Is(err, taskauthzext.ErrUnauthenticated):
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
	case errors.Is(err, taskauthzext.ErrForbidden):
		slog.WarnContext(r.Context(), "tasks: authorization denied", append(logAttrs, "error", err)...)
		httputil.Error(w, r, http.StatusForbidden, errForbiddenTaskAction)
	default:
		httputil.InternalServerError(w, r, "tasks: failed to complete task step", err, logAttrs...)
	}
}

// completeTaskStepRequest is the JSON envelope both completion routes accept:
// {"command": "...", "payload": {...}}. Payload stays map[string]any because its
// contents are genuinely dynamic per task type; only the envelope around it has
// a fixed shape.
type completeTaskStepRequest struct {
	Command string         `json:"command"`
	Payload map[string]any `json:"payload"`
}
