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
//	POST /api/v1/tasks/{id}
//	body: arbitrary JSON object — passed through to the task plugin
func (h *HTTPHandler) HandleCompleteTaskStep(w http.ResponseWriter, r *http.Request) {
	// TODO: retrieve the authenticated context and validate it against the
	// task's ownership bounds before completing the step.
	taskID := r.PathValue("id")
	command := r.PathValue("command")

	var rawBody map[string]any
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		// An empty body is a valid acknowledge-style completion; only fail on
		// genuinely malformed JSON.
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyReadAfterClose) {
			writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			slog.Error("tasks: failed to decode request body", "error", err)
			return
		}
	}

	// 1. Validate taskID (guaranteed by URL routing rules)
	var payload map[string]any

	// 2. Resolve command & payload
	if command != "" {
		// URL-based route: body is the flat payload
		payload = rawBody
	} else {
		// Body-based route: body must be the nested envelope containing "command" and "payload"
		if rawBody == nil {
			writeJSONError(w, http.StatusBadRequest, "request body is required for body-based command route")
			slog.Error("tasks: missing request body for body-based command", "taskId", taskID)
			return
		}

		cmd, hasCmd := rawBody["command"].(string)
		p, ok := rawBody["payload"].(map[string]any)

		if !hasCmd || !ok {
			writeJSONError(w, http.StatusBadRequest, "invalid request body: must be an envelope containing 'command' (string) and 'payload' (object)")
			slog.Error("tasks: invalid body-based command envelope", "taskId", taskID, "hasCmd", hasCmd, "hasPayload", ok)
			return
		}

		command = cmd
		payload = p
	}

	// 3. Validate system metadata collision
	if payload != nil {
		if _, exists := payload["__command"]; exists {
			writeJSONError(w, http.StatusBadRequest, "invalid request payload: '__command' is a reserved system key")
			slog.Error("tasks: reserved key '__command' present in payload", "taskId", taskID)
			return
		}
	}

	if payload == nil {
		payload = make(map[string]any)
	}

	payload["__command"] = command

	slog.Info("tasks: processing complete step command", "taskId", taskID, "command", command)

	if err := h.Manager.CompleteTaskStep(r.Context(), taskID, payload); err != nil {
		slog.Error("tasks: failed to complete task step", "taskId", taskID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "An internal error occurred while processing the task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
