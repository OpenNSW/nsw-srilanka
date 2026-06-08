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

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		// An empty body is a valid acknowledge-style completion; only fail on
		// genuinely malformed JSON.
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyReadAfterClose) {
			writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			slog.Error("tasks: failed to decode request body", "error", err)
			return
		}
	}

	// TODO(oga-callback): drop the body-id fallback once OGA POSTs directly
	// to /api/v1/tasks/{id}. For now the legacy POST /api/v1/tasks route also
	// wires here, and OGA's envelope carries task_id in the body.
	if taskID == "" {
		if id, ok := payload["task_id"].(string); ok {
			taskID = id
		}
	}
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task id is required")
		slog.Error("tasks: missing task id in request")
		return
	}

	payload = unwrapOGACallback(payload)

	if err := h.Manager.CompleteTaskStep(r.Context(), taskID, payload); err != nil {
		slog.Error("tasks: failed to complete task step", "taskId", taskID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "An internal error occurred while processing the task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unwrapOGACallback detects OGA's legacy TaskResponse envelope and returns the
// reviewer payload that the task plugin actually expects.
//
// Agency's sendToService posts back:
//
//	{ "task_id": "...", "consignment_id": "...", "payload": { "action": "AGENCY_VERIFICATION", "content": {...} } }
//
// but our plugins expect the bare reviewer form data. We detect the envelope
// (presence of task_id + consignment_id + payload.content) and lift content up.
//
// TODO(agency-callback): remove once the agency is updated to post the bare reviewer
// payload directly to /api/v1/tasks/{id} (or to a dedicated /oga-callback
// route that owns this translation).
func unwrapOGACallback(payload map[string]any) map[string]any {
	if payload == nil {
		return payload
	}
	if _, hasTaskID := payload["task_id"]; !hasTaskID {
		return payload
	}
	if _, hasConsignmentID := payload["consignment_id"]; !hasConsignmentID {
		return payload
	}
	envelope, ok := payload["payload"].(map[string]any)
	if !ok {
		return payload
	}
	content, ok := envelope["content"].(map[string]any)
	if !ok {
		return payload
	}
	slog.Info("tasks: unwrapped OGA callback envelope", "action", envelope["action"])
	return content
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
