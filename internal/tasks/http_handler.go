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

	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"

	taskauthz "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
)

// TaskFetcher is the narrow surface HandleGetTask needs from the task store.
type TaskFetcher interface {
	GetTask(ctx context.Context, taskID string) (store.TaskRecord, bool)
}

type HTTPHandler struct {
	Manager   *orchestrator.TaskManager
	Store     TaskFetcher
	Assembler *zoneview.ZoneViewAssembler
}

func NewHTTPHandler(manager *orchestrator.TaskManager, store TaskFetcher, assembler *zoneview.ZoneViewAssembler) *HTTPHandler {
	return &HTTPHandler{Manager: manager, Store: store, Assembler: assembler}
}

// HandleGetTask returns the ZoneView payload for a single task.
//
//	GET /api/v1/tasks/{id}
func (h *HTTPHandler) HandleGetTask(w http.ResponseWriter, r *http.Request) {
	// TODO: retrieve the authenticated context and validate it against the
	// task's ownership bounds before returning ZoneView.
	taskID := r.PathValue("id")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task id is required")
		return
	}

	record, ok := h.Store.GetTask(r.Context(), taskID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "task not found")
		return
	}

	zv, err := h.Assembler.Assemble(r.Context(), record)
	if err != nil {
		slog.Error("tasks: failed to assemble zone view", "taskId", taskID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "An internal error occurred while loading the task")
		return
	}

	writeJSONResponse(w, http.StatusOK, zv)
}

// HandleCompleteTaskStep advances a task by submitting a step payload.
//
//	POST /api/v1/tasks/{id}/commands/{command}
//	POST /api/v1/tasks/{id}
func (h *HTTPHandler) HandleCompleteTaskStep(w http.ResponseWriter, r *http.Request) {
	// TODO: retrieve the authenticated context and validate it against the
	// task's ownership bounds before completing the step.
	taskID := r.PathValue("id")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task id is required")
		slog.Error("tasks: missing task id in request")
		return
	}

	pathCommand := r.PathValue("command")

	command, payload, status, err := parseCompleteTaskStepRequest(r, pathCommand)
	if err != nil {
		writeJSONError(w, status, err.Error())
		slog.Error("tasks: failed to parse request", "taskId", taskID, "error", err)
		return
	}

	slog.Info("tasks: processing complete step command", "taskId", taskID, "command", command)

	if err := h.Manager.CompleteTaskStep(r.Context(), taskID, payload); err != nil {
		switch {
		case errors.Is(err, taskauthz.ErrUnauthenticated):
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
		case errors.Is(err, taskauthz.ErrForbidden):
			slog.Warn("tasks: authorization denied", "taskId", taskID, "command", command, "error", err)
			writeJSONError(w, http.StatusForbidden, "you may not perform this action on this task")
		default:
			slog.Error("tasks: failed to complete task step", "taskId", taskID, "error", err)
			writeJSONError(w, http.StatusInternalServerError, "An internal error occurred while processing the task")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseCompleteTaskStepRequest extracts and validates the command and payload from either the URL path or the JSON body.
func parseCompleteTaskStepRequest(r *http.Request, command string) (string, map[string]any, int, error) {
	var rawBody map[string]any
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		// An empty body is a valid acknowledge-style completion; only fail on genuinely malformed JSON.
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyReadAfterClose) {
			return "", nil, http.StatusBadRequest, errors.New("invalid request body: " + err.Error())
		}
	}

	var payload map[string]any

	if command != "" {
		// URL-based route: body is the flat payload
		payload = rawBody
	} else {
		// Body-based route: body must be the nested envelope containing "command" and "payload"
		if rawBody == nil {
			return "", nil, http.StatusBadRequest, errors.New("request body is required for body-based command route")
		}

		cmd, hasCmd := rawBody["command"].(string)
		if !hasCmd {
			return "", nil, http.StatusBadRequest, errors.New("invalid request body: must contain 'command' (string)")
		}

		var p map[string]any
		if rawBody["payload"] != nil {
			var ok bool
			p, ok = rawBody["payload"].(map[string]any)
			if !ok {
				return "", nil, http.StatusBadRequest, errors.New("invalid request body: 'payload' must be an object")
			}
		}

		command = cmd
		payload = p
	}

	// Validate system metadata collision
	if payload != nil {
		if _, exists := payload["__command"]; exists {
			return "", nil, http.StatusBadRequest, errors.New("invalid request payload: '__command' is a reserved system key")
		}
	}

	if payload == nil {
		payload = make(map[string]any)
	}

	payload["__command"] = command

	return command, payload, http.StatusOK, nil
}

func writeJSONResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("tasks: failed to encode JSON response", "error", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSONResponse(w, status, map[string]string{"error": message})
}
