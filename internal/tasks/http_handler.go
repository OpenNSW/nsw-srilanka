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
	errCommandNotOffered   = "this step is no longer waiting for that action"
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
	// from their role-tied ownership of the task's consignment.
	if err := readauthz.Authorize(ctx, h.AuthzCatalog, in, record.RootWorkflowID); err != nil {
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

	zv, err := h.Assembler.Assemble(ctx, record, nil)
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
	taskID := r.PathValue("id")
	if taskID == "" {
		slog.ErrorContext(r.Context(), "tasks: missing task id in request")
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	pathCommand := r.PathValue("command")

	command, payload, status, responseMessage, err := parseCompleteTaskStepRequest(r, pathCommand)
	if err != nil {
		slog.ErrorContext(r.Context(), "tasks: failed to parse request", "taskId", taskID, "error", err)
		httputil.Error(w, r, status, responseMessage)
		return
	}

	slog.InfoContext(r.Context(), "tasks: processing complete step command", "taskId", taskID, "command", command)

	// A command the task's current state does not offer is refused here rather
	// than passed on. CompleteTaskStep writes the payload into whichever
	// subtask is active *before* it tries to resume the workflow, so a command
	// arriving after the step it belongs to has moved on — a page reloaded
	// mid-call, a second tab, a retried request — replaces the output
	// namespace of the subtask that has since taken over, discarding what that
	// subtask recorded there. The ZoneView assembler already hides a handle
	// its state does not list; this applies the same rule to the endpoint, so
	// hiding the button is not the only thing standing between a stale click
	// and another step's results.
	if command != "" && h.Store != nil {
		record, ok := h.Store.GetTask(r.Context(), taskID)
		if !ok {
			httputil.Error(w, r, http.StatusNotFound, errTaskNotFound)
			return
		}
		if !offersCommand(record, command) {
			slog.WarnContext(r.Context(), "tasks: refusing a command the current state does not offer",
				"taskId", taskID, "command", command, "state", record.State, "active_template", record.ActiveTaskTemplateID)
			httputil.Error(w, r, http.StatusConflict, errCommandNotOffered)
			return
		}
	}

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

// offersCommand reports whether the task's current state lists this command
// among its actions — the same states[<state>].actions contract the ZoneView
// assembler filters handles by.
//
// It answers yes wherever the contract is silent (no render config, a config
// that declares no states, or a state it does not describe): those flows never
// stated which commands belong to which state, so the orchestrator remains the
// judge and behaviour is unchanged. Only a state that does describe itself can
// refuse — including one whose action list is deliberately empty, which is how
// a render config says "nothing to do here but wait".
func offersCommand(record store.TaskRecord, command string) bool {
	if len(record.RenderConfig) == 0 {
		return true
	}
	var cfg zoneview.TaskTemplateConfig
	if err := json.Unmarshal(record.RenderConfig, &cfg); err != nil {
		return true
	}
	state, described := cfg.States[record.State]
	if !described {
		return true
	}
	for _, a := range state.Actions {
		if a.Command == command {
			return true
		}
	}
	return false
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
