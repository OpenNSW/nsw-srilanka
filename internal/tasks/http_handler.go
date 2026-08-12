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
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	taskauthz "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
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
	Manager         *orchestrator.TaskManager
	Store           TaskFetcher
	Assembler       *zoneview.ZoneViewAssembler
	MaxRequestBytes int64
	audit           *nswaudit.Recorder
}

func NewHTTPHandler(manager *orchestrator.TaskManager, store TaskFetcher, assembler *zoneview.ZoneViewAssembler, maxRequestBytes int64, recorder *nswaudit.Recorder) *HTTPHandler {
	return &HTTPHandler{Manager: manager, Store: store, Assembler: assembler, MaxRequestBytes: maxRequestBytes, audit: recorder}
}

// HandleGetTask returns the ZoneView payload for a single task.
//
//	GET /api/v1/tasks/{id}
func (h *HTTPHandler) HandleGetTask(w http.ResponseWriter, r *http.Request) {
	// TODO: retrieve the authenticated context and validate it against the
	// task's ownership bounds before returning ZoneView.
	taskID := r.PathValue("id")
	if taskID == "" {
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}

	record, ok := h.Store.GetTask(r.Context(), taskID)
	if !ok {
		httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
		return
	}

	zv, err := h.Assembler.Assemble(r.Context(), record, nil)
	if err != nil {
		httputil.InternalServerError(w, r, "tasks: failed to assemble zone view", err, "taskId", taskID)
		return
	}

	httputil.JSON(w, http.StatusOK, zv)
}

// HandleCompleteTaskStep advances a task by submitting a step payload.
//
//	POST /api/v1/tasks/{id}/commands/{command}
//	POST /api/v1/tasks/{id}
func (h *HTTPHandler) HandleCompleteTaskStep(w http.ResponseWriter, r *http.Request) {
	// TODO: retrieve the authenticated context and validate it against the
	// task's ownership bounds before completing the step.
	ctx := r.Context()
	taskID := r.PathValue("id")
	if taskID == "" {
		slog.ErrorContext(ctx, "tasks: missing task id in request")
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	pathCommand := r.PathValue("command")

	command, payload, status, responseMessage, err := parseCompleteTaskStepRequest(r, pathCommand)
	if err != nil {
		slog.ErrorContext(ctx, "tasks: failed to parse request", "taskId", taskID, "error", err)
		auditCommand := command
		if auditCommand == "" {
			auditCommand = pathCommand
		}
		h.audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventTask,
			Action:     nswaudit.ActionUpdate,
			TargetType: nswaudit.TargetTask,
			TargetID:   taskID,
			Failure:    true,
			Metadata: map[string]any{
				"status":  status,
				"command": auditCommand,
				"error":   responseMessage,
			},
		})
		httputil.Error(w, r, status, responseMessage)
		return
	}

	slog.InfoContext(ctx, "tasks: processing complete step command", "taskId", taskID, "command", command)

	if err := h.Manager.CompleteTaskStep(ctx, taskID, payload); err != nil {
		var httpStatus int
		switch {
		case errors.Is(err, taskauthz.ErrUnauthenticated):
			httpStatus = http.StatusUnauthorized
			httputil.Error(w, r, httpStatus, errAuthenticationReq)
		case errors.Is(err, taskauthz.ErrForbidden):
			httpStatus = http.StatusForbidden
			slog.WarnContext(ctx, "tasks: authorization denied", "taskId", taskID, "command", command, "error", err)
			httputil.Error(w, r, httpStatus, errForbiddenTaskAction)
		default:
			httpStatus = http.StatusInternalServerError
			httputil.InternalServerError(w, r, "tasks: failed to complete task step", err, "taskId", taskID)
		}
		h.audit.Record(ctx, nswaudit.Event{
			EventType:  nswaudit.EventTask,
			Action:     nswaudit.ActionUpdate,
			TargetType: nswaudit.TargetTask,
			TargetID:   taskID,
			Failure:    true,
			Metadata: map[string]any{
				"status":  httpStatus,
				"command": command,
				"error":   err.Error(),
			},
		})
		return
	}

	h.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventTask,
		Action:     nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetTask,
		TargetID:   taskID,
		Failure:    false,
		Metadata: map[string]any{
			"command": command,
			"status":  http.StatusNoContent,
		},
	})
	w.WriteHeader(http.StatusNoContent)
}

// parseCompleteTaskStepRequest extracts and validates the command and payload from either the URL path or the JSON body.
// The body must contain at most one JSON value: json.Decoder.Decode only parses the first value and
// silently ignores anything after it, so a second Decode call is required to confirm nothing trails it.
func parseCompleteTaskStepRequest(r *http.Request, command string) (string, map[string]any, int, string, error) {
	var rawBody map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&rawBody); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return "", nil, http.StatusRequestEntityTooLarge, errRequestBodyTooLarge, err
		}

		// An empty body is a valid acknowledge-style completion; only fail on genuinely malformed JSON.
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyReadAfterClose) {
			slog.WarnContext(r.Context(), "tasks: malformed request body", "error", err)
			return "", nil, http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: malformed JSON")
		}

		// If unexpected data follows the first JSON value, reject the request.
	} else if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return "", nil, http.StatusRequestEntityTooLarge, errRequestBodyTooLarge, err
		}
		slog.WarnContext(r.Context(), "tasks: unexpected data after JSON body", "error", err)
		return "", nil, http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: unexpected data after JSON value")
	}

	var payload map[string]any

	if command != "" {
		// URL-based route: body is the flat payload
		payload = rawBody
	} else {
		// Body-based route: body must be the nested envelope containing "command" and "payload"
		if rawBody == nil {
			return "", nil, http.StatusBadRequest, errInvalidRequestBody, errors.New("request body is required for body-based command route")
		}

		cmd, hasCmd := rawBody["command"].(string)
		if !hasCmd {
			return "", nil, http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: must contain 'command' (string)")
		}

		var p map[string]any
		if rawBody["payload"] != nil {
			var ok bool
			p, ok = rawBody["payload"].(map[string]any)
			if !ok {
				return "", nil, http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: 'payload' must be an object")
			}
		}

		command = cmd
		payload = p
	}

	// Validate system metadata collision
	if payload != nil {
		if _, exists := payload["__command"]; exists {
			return "", nil, http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request payload: '__command' is a reserved system key")
		}
	}

	if payload == nil {
		payload = make(map[string]any)
	}

	payload["__command"] = command

	return command, payload, http.StatusOK, "", nil
}
