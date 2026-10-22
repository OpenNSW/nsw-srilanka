// Package tasks hosts the HTTP surface for the core-based task orchestrator
// (the core/taskflow port of the old internal/taskv2 HTTP handler).
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/payloadschema"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/filefields"
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
	errFileNotReference     = "a file in the submission is not a file reference; upload it again"
	errFileNotYours         = "a file in the submission was issued to someone else"
	errFileExpired          = "a file in the submission has expired; refetch the task and submit again"
	errFileScope            = "a file in the submission can't be submitted; refetch the task"
	errFileShape            = "a file field in the submission does not have the shape the task declares"
	errDraftNotEnabled      = "this step does not accept drafts"
	errInvalidPayload       = "submission does not match the form"
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
	Audit           nswaudit.Auditor
	MaxRequestBytes int64
	// Files swaps the task's declared file fields: stored values out as
	// references for a reader, references in as stored values on a submission.
	Files *FileBinding
	// CanAct decides whether a reader's file references can be submitted.
	// Without it, they can only be opened.
	CanAct CanAct
}

func NewHTTPHandler(
	manager *orchestrator.TaskManager,
	store TaskFetcher,
	assembler *zoneview.ZoneViewAssembler,
	authzCatalog taskauthz.Catalog,
	files *FileBinding,
	canAct CanAct,
	audit nswaudit.Auditor,
	maxRequestBytes int64,
) *HTTPHandler {
	return &HTTPHandler{
		Manager:         manager,
		Store:           store,
		Assembler:       assembler,
		AuthzCatalog:    authzCatalog,
		Files:           files,
		CanAct:          canAct,
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

	// The case ID is the consignment id, so this decides the caller's access
	// from their role-tied ownership of the task's consignment, and returns the
	// claims that shape their view of it.
	claims, err := readauthz.Resolve(ctx, h.AuthzCatalog, in, record.RenderConfig, record.CaseID)
	if err != nil {
		if !errors.Is(err, readauthz.ErrDenied) {
			httputil.InternalServerError(w, r, "tasks: failed to resolve read access", err, "taskId", taskID)
			return
		}
		// Answer with the not-found status and text, so a denied read is
		// indistinguishable from a task that does not exist and cannot be used to
		// probe which task ids are real. Mirrors GET /api/v1/consignments/{id}.
		slog.WarnContext(ctx, "tasks: read authorization denied", "taskId", taskID)
		h.auditEvent(ctx, nswaudit.Event{
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

	// The reader gets references to the task's files, never their stored
	// values, and can submit them back only on a step they may act on.
	p, _ := authn.FromContext(ctx)
	view, err := h.Files.ForReader(record, p, h.canAct(ctx, record, in))
	if err != nil {
		httputil.InternalServerError(w, r, "tasks: failed to issue file references", err, "taskId", taskID)
		return
	}

	zv, err := h.Assembler.Assemble(ctx, view, claims)
	if err != nil {
		httputil.InternalServerError(w, r, "tasks: failed to assemble zone view", err, "taskId", taskID)
		return
	}

	httputil.JSON(w, http.StatusOK, zv)
}

// canAct reports whether in may act on record's active step. A failure to
// decide is logged and taken as no: the reader can still open the task's
// files, and refetches the task to submit them.
func (h *HTTPHandler) canAct(ctx context.Context, record store.TaskRecord, in taskauthz.Input) bool {
	if h.CanAct == nil {
		return false
	}
	ok, err := h.CanAct(ctx, record, in)
	if err != nil {
		slog.ErrorContext(ctx, "tasks: failed to decide whether the reader can act; their file references can only be opened", "taskId", record.TaskID, "error", err)
		return false
	}
	return ok
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
	if !ok || !h.resolveFiles(w, r, taskID, stepID, payload) {
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
		h.auditTaskUpdate(r.Context(), "", true, taskAuditMeta(http.StatusBadRequest, "invalid_callback_token"))
		httputil.Error(w, r, http.StatusBadRequest, errInvalidCallbackToken)
		return
	}

	payload, ok := h.decodeSubmission(w, r, "taskId", taskID, "stepId", stepID)
	if !ok || !h.resolveFiles(w, r, taskID, stepID, payload) {
		return
	}
	// Read before the call: core may strip system keys from the payload.
	command := payload["__command"]
	err = h.Manager.CompleteTaskStepByToken(r.Context(), token, payload)
	h.writeCompletion(w, r, err, "taskId", taskID, "stepId", stepID, "command", command)
}

// HandleSaveDraft stores a draft of a step without completing it. It is the
// portal's route for a form's "save as draft" action.
//
//	PUT /api/v1/tasks/{id}/steps/{stepId}/draft
//
// The body is {"payload": {...}}, the same form object a submission carries. It
// takes no command: core authorizes the draft as the command the step's template
// declares for drafts. Core stores the payload under the step's own namespace, and
// the step, its id and the task's version stay as they are, so the view the caller
// holds stays current. 204 when the draft is stored; 400 when the step does not
// accept drafts; 409 when the step has been submitted or the task moved on.
func (h *HTTPHandler) HandleSaveDraft(w http.ResponseWriter, r *http.Request) {
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

	req, ok := h.decodeEnvelope(w, r, "taskId", taskID, "stepId", stepID)
	if !ok {
		return
	}
	if req.Command != "" {
		slog.ErrorContext(r.Context(), "tasks: draft request names a command", "taskId", taskID, "stepId", stepID, "command", req.Command)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestBody)
		return
	}
	payload := req.Payload
	if payload == nil {
		payload = make(map[string]any)
	}
	if !h.resolveFiles(w, r, taskID, stepID, payload) {
		return
	}
	// Marked as a draft save so the payload schema extension knows a draft by the
	// route it came through, not by the command core sets for it.
	err := h.Manager.SaveDraft(payloadschema.WithDraft(r.Context()), taskID, stepID, payload)
	h.writeCompletion(w, r, err, "taskId", taskID, "stepId", stepID, "draft", true)
}

// decodeSubmission reads the {command, payload} envelope both completion routes take
// and returns the payload with the command set under the reserved "__command" key.
// On a bad request it writes the error response and returns false. logAttrs name
// the step in the log lines.
func (h *HTTPHandler) decodeSubmission(w http.ResponseWriter, r *http.Request, logAttrs ...any) (map[string]any, bool) {
	req, ok := h.decodeEnvelope(w, r, logAttrs...)
	if !ok {
		return nil, false
	}

	if req.Command == "" {
		slog.ErrorContext(r.Context(), "tasks: failed to parse request", append(logAttrs, "error", errors.New("invalid request body: must contain 'command' (string)"))...)
		httputil.Error(w, r, http.StatusBadRequest, errInvalidRequestBody)
		return nil, false
	}

	payload := req.Payload
	if payload == nil {
		payload = make(map[string]any)
	}

	payload[orchestrator.CommandVar] = req.Command

	slog.InfoContext(r.Context(), "tasks: processing complete step command", append(logAttrs, "command", req.Command)...)
	return payload, true
}

// decodeEnvelope reads the {command, payload} envelope the completion and draft
// routes take: one JSON value within the size limit, whose payload does not set
// the reserved "__command" key. On a bad request it writes the error response and
// returns false. logAttrs name the step in the log lines.
func (h *HTTPHandler) decodeEnvelope(w http.ResponseWriter, r *http.Request, logAttrs ...any) (completeTaskStepRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	ctx := r.Context()
	var req completeTaskStepRequest
	fail := func(status int, message string, err error) {
		slog.ErrorContext(ctx, "tasks: failed to parse request", append(logAttrs, "error", err)...)
		meta := taskAuditMeta(status, message, logAttrs...)
		if req.Command != "" {
			meta["command"] = req.Command
		}
		h.auditTaskUpdate(ctx, attrString(logAttrs, "taskId"), true, meta)
		httputil.Error(w, r, status, message)
	}

	// The body must contain at most one JSON value: json.Decoder.Decode only parses the
	// first value and silently ignores anything after it, so a second Decode call is
	// required to confirm nothing trails it.
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fail(http.StatusRequestEntityTooLarge, errRequestBodyTooLarge, err)
			return completeTaskStepRequest{}, false
		}

		// An empty body is tolerated here; a route that needs a command checks for one.
		// Only fail on genuinely malformed JSON.
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyReadAfterClose) {
			fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: malformed JSON"))
			return completeTaskStepRequest{}, false
		}

		// If unexpected data follows the first JSON value, reject the request.
	} else if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fail(http.StatusRequestEntityTooLarge, errRequestBodyTooLarge, err)
			return completeTaskStepRequest{}, false
		}
		fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request body: unexpected data after JSON value"))
		return completeTaskStepRequest{}, false
	}

	// Validate system metadata collision
	if _, exists := req.Payload[orchestrator.CommandVar]; exists {
		fail(http.StatusBadRequest, errInvalidRequestBody, errors.New("invalid request payload: '__command' is a reserved system key"))
		return completeTaskStepRequest{}, false
	}
	return req, true
}

// resolveFiles replaces the file references in a submission for step stepID
// of task taskID with the stored values they stand for, so only those are
// stored. On failure it writes the response and returns false.
func (h *HTTPHandler) resolveFiles(w http.ResponseWriter, r *http.Request, taskID, stepID string, payload map[string]any) bool {
	ctx := r.Context()
	record, ok := h.Store.GetTask(ctx, taskID)
	if !ok {
		h.rejectFiles(w, r, taskID, stepID, http.StatusNotFound, errTaskNotFound, "task_not_found")
		return false
	}
	// Which fields hold files depends on the step. Core rejects a submission
	// for a step that is not active anyway; do it first, before resolving
	// anything against the wrong step.
	if record.ActiveStepID != stepID {
		err := fmt.Errorf("%w: call was for step %q, active step is %q", orchestrator.ErrStaleStep, stepID, record.ActiveStepID)
		h.writeCompletion(w, r, err, "taskId", taskID, "stepId", stepID)
		return false
	}

	p, _ := authn.FromContext(ctx)
	err := h.Files.FromCaller(ctx, record, payload, p)
	switch {
	case err == nil:
		return true
	case errors.Is(err, fileaccess.ErrInvalid):
		slog.WarnContext(ctx, "tasks: submission carries a file value that is not a file reference", "taskId", taskID, "stepId", stepID)
		h.rejectFiles(w, r, taskID, stepID, http.StatusBadRequest, errFileNotReference, "file_not_reference")
	case errors.Is(err, fileaccess.ErrNotYours):
		slog.WarnContext(ctx, "tasks: submission carries a file issued to someone else", "taskId", taskID, "stepId", stepID, "error", err)
		h.rejectFiles(w, r, taskID, stepID, http.StatusForbidden, errFileNotYours, "file_not_yours")
	case errors.Is(err, fileaccess.ErrScope):
		slog.WarnContext(ctx, "tasks: submission carries a file that can't be submitted", "taskId", taskID, "stepId", stepID, "error", err)
		h.rejectFiles(w, r, taskID, stepID, http.StatusForbidden, errFileScope, "file_scope")
	case errors.Is(err, fileaccess.ErrExpired):
		h.rejectFiles(w, r, taskID, stepID, http.StatusGone, errFileExpired, "file_expired")
	case errors.Is(err, filefields.ErrShape):
		slog.WarnContext(ctx, "tasks: submission has a file field of the wrong shape", "taskId", taskID, "stepId", stepID, "error", err)
		h.rejectFiles(w, r, taskID, stepID, http.StatusBadRequest, errFileShape, "file_shape")
	default:
		h.auditTaskUpdate(ctx, taskID, true, taskAuditMeta(http.StatusInternalServerError, "file_resolve_failed", "stepId", stepID))
		httputil.InternalServerError(w, r, "tasks: failed to resolve file references", err, "taskId", taskID, "stepId", stepID)
	}
	return false
}

func (h *HTTPHandler) rejectFiles(w http.ResponseWriter, r *http.Request, taskID, stepID string, status int, public, code string) {
	h.auditTaskUpdate(r.Context(), taskID, true, taskAuditMeta(status, code, "stepId", stepID))
	httputil.Error(w, r, status, public)
}

// writeCompletion answers a completion attempt or a draft save: 204 when the workflow accepted the
// step, otherwise the error mapped to its status. logAttrs name the step.
func (h *HTTPHandler) writeCompletion(w http.ResponseWriter, r *http.Request, err error, logAttrs ...any) {
	ctx := r.Context()
	taskID := attrString(logAttrs, "taskId")
	finish := func(status int, failure bool, code string) {
		h.auditTaskUpdate(ctx, taskID, failure, taskAuditMeta(status, code, logAttrs...))
	}

	switch {
	case err == nil:
		finish(http.StatusNoContent, false, "")
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, orchestrator.ErrStaleStep):
		slog.InfoContext(ctx, "tasks: stale step", append(logAttrs, "error", err)...)
		finish(http.StatusConflict, true, "stale_step")
		httputil.Error(w, r, http.StatusConflict, errStaleStep)
	case errors.Is(err, taskauthzext.ErrUnauthenticated):
		finish(http.StatusUnauthorized, true, "unauthenticated")
		httputil.Error(w, r, http.StatusUnauthorized, errAuthenticationReq)
	case errors.Is(err, taskauthzext.ErrForbidden):
		slog.WarnContext(ctx, "tasks: authorization denied", append(logAttrs, "error", err)...)
		finish(http.StatusForbidden, true, "forbidden")
		httputil.Error(w, r, http.StatusForbidden, errForbiddenTaskAction)
	case errors.Is(err, orchestrator.ErrStepNotGuarded):
		// The step's template names no one who may act, so no one may. That is a
		// misconfigured template, not a caller's mistake, so it is logged as an error.
		slog.ErrorContext(r.Context(), "tasks: step declares no authz rule", append(logAttrs, "error", err)...)
		httputil.Error(w, r, http.StatusForbidden, errForbiddenTaskAction)
	case errors.Is(err, payloadschema.ErrInvalidPayload):
		// The browser checks the same schema first, so this is a form the browser and
		// server disagree on, or a call that skipped the browser. The failing fields
		// go to the log only.
		slog.WarnContext(r.Context(), "tasks: submission does not match its form", append(logAttrs, "error", err)...)
		finish(http.StatusBadRequest, true, "invalid_payload")
		httputil.Error(w, r, http.StatusBadRequest, errInvalidPayload)
	case errors.Is(err, orchestrator.ErrDraftNotEnabled):
		slog.WarnContext(r.Context(), "tasks: draft for a step that takes none", append(logAttrs, "error", err)...)
		finish(http.StatusBadRequest, true, "draft_not_enabled")
		httputil.Error(w, r, http.StatusBadRequest, errDraftNotEnabled)
	default:
		finish(http.StatusInternalServerError, true, "complete_failed")
		httputil.InternalServerError(w, r, "tasks: failed to complete task step", err, logAttrs...)
	}
}

func (h *HTTPHandler) auditTaskUpdate(ctx context.Context, taskID string, failure bool, meta map[string]any) {
	h.auditEvent(ctx, nswaudit.Event{
		EventType:  nswaudit.EventTask,
		Action:     nswaudit.ActionUpdate,
		TargetType: nswaudit.TargetTask,
		TargetID:   taskID,
		Failure:    failure,
		Metadata:   meta,
	})
}

// taskAuditMeta copies the step, command, and draft flag out of a slog-style
// attr list. Empty command is omitted: a draft save has no command, and a
// stale-step rejection from file resolution does not either.
func taskAuditMeta(status int, errCode string, logAttrs ...any) map[string]any {
	meta := map[string]any{"status": status}
	if cmd := attrString(logAttrs, "command"); cmd != "" {
		meta["command"] = cmd
	}
	if step := attrString(logAttrs, "stepId"); step != "" {
		meta["stepId"] = step
	}
	if draft, ok := attrBool(logAttrs, "draft"); ok && draft {
		meta["draft"] = true
	}
	if errCode != "" {
		meta["error"] = errCode
	}
	return meta
}

func (h *HTTPHandler) auditEvent(ctx context.Context, e nswaudit.Event) {
	if h.Audit != nil {
		h.Audit.Audit(ctx, e)
	}
}

// attrString returns the string value for key in a slog-style attr list.
func attrString(attrs []any, key string) string {
	for i := 0; i+1 < len(attrs); i += 2 {
		k, ok := attrs[i].(string)
		if !ok || k != key {
			continue
		}
		switch v := attrs[i+1].(type) {
		case string:
			return v
		case fmt.Stringer:
			return v.String()
		default:
			if v == nil {
				return ""
			}
			return fmt.Sprint(v)
		}
	}
	return ""
}

func attrBool(attrs []any, key string) (bool, bool) {
	for i := 0; i+1 < len(attrs); i += 2 {
		k, ok := attrs[i].(string)
		if !ok || k != key {
			continue
		}
		v, ok := attrs[i+1].(bool)
		return v, ok
	}
	return false, false
}

// completeTaskStepRequest is the JSON envelope the completion routes accept:
// {"command": "...", "payload": {...}}. The draft route takes it without the
// command. Payload stays map[string]any because its contents are genuinely
// dynamic per task type; only the envelope around it has a fixed shape.
type completeTaskStepRequest struct {
	Command string         `json:"command"`
	Payload map[string]any `json:"payload"`
}
