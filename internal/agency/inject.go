// Package agency lets this backend act as a government agency: external systems
// inject applications through POST /api/v1/inject, each taskId starts its own
// workflow on the shared engine, and officers work the resulting tasks through the
// normal /api/v1/tasks/{id} routes. It is kept out of the TNSW packages on purpose
// — see docs/agency.md.
package agency

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
	"github.com/OpenNSW/core/httputil"
	workflow "github.com/OpenNSW/core/workflow"

	"github.com/OpenNSW/nsw-srilanka/internal/agency/taskconfig/taskconfigart"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/filefields"
)

// ScopeWorkflowInject gates POST /api/v1/inject. An inject starts one workflow (and
// creates or joins its case as a side effect), so the scope names the workflow. It is
// an M2M permission, kept apart from the officer's nsw:task:* scopes.
const ScopeWorkflowInject = "nsw:workflow:inject"

const (
	errInvalidBody        = "invalid request body"
	errBodyTooLarge       = "request body too large"
	errInvalidFiles       = "a file in the payload is not one this client may send"
	errTaskIDRequired     = "taskId is required"
	errTaskCodeRequired   = "taskCode is required"
	errConsignmentIDReq   = "consignmentId is required"
	errFailedToInjectTask = "failed to inject task"
)

// ErrUnknownTaskCode is returned by Inject when no task_config is registered for the
// request's taskCode.
var ErrUnknownTaskCode = errors.New("unknown task code")

// ErrInvalidFiles is returned by Inject when a file the task config declares in the
// payload is not one the caller may send.
var ErrInvalidFiles = errors.New("a file in the payload is not one this client may send")

// ErrConflict is returned by Inject when taskId is already recorded with a different
// consignmentId or taskCode.
var ErrConflict = errors.New("taskId already injected with different details")

// InjectRequest is the request body for POST /api/v1/inject.
type InjectRequest struct {
	TaskID        string `json:"taskId"`
	TaskCode      string `json:"taskCode"`
	ConsignmentID string `json:"consignmentId"`
	// CallbackToken is the opaque token the caller expects the decision back on. It
	// names the caller's step, so the workflow hands it back unchanged.
	CallbackToken string          `json:"callbackToken"`
	Data          json.RawMessage `json:"data"`
}

// Service starts one workflow per injected taskId, running the workflow the
// taskCode's task_config names.
type Service struct {
	repo             Repository
	artifactRegistry *artifact.Registry
	wm               workflow.Manager
	files            FileAcceptor
	maxBodyBytes     int64
}

// FileAcceptor decides what is stored for a file value the caller sent in a payload:
// *fileaccess.Access satisfies it, and its errors are the ones Inject maps.
type FileAcceptor interface {
	Accept(p *authn.Principal, ref string) (string, error)
}

// Option configures a Service.
type Option func(*Service)

// WithFiles sets how the files a task config declares in an injected payload are
// stored. Without it, a task config that declares files cannot be injected.
func WithFiles(files FileAcceptor) Option {
	return func(s *Service) { s.files = files }
}

// WithMaxBodyBytes limits the request body HandleInject reads.
func WithMaxBodyBytes(n int64) Option {
	return func(s *Service) { s.maxBodyBytes = n }
}

// NewService creates a Service.
func NewService(repo Repository, artifactRegistry *artifact.Registry, wm workflow.Manager, opts ...Option) *Service {
	s := &Service{repo: repo, artifactRegistry: artifactRegistry, wm: wm}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Inject records req and starts its workflow unless it has already started.
// It is safe to retry: a row left in StatusStarting by a failed start is started
// again, and a start racing another for the same taskId is deduplicated by the
// engine, which returns the running execution instead of starting a second one.
func (s *Service) Inject(ctx context.Context, req InjectRequest) (*Workflow, error) {
	// Resolved before anything is recorded, so an unknown taskCode leaves no rows.
	cfg, err := taskconfigart.Load(ctx, s.artifactRegistry, req.TaskCode)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			return nil, fmt.Errorf("%w %q", ErrUnknownTaskCode, req.TaskCode)
		}
		return nil, fmt.Errorf("agency: failed to load task config %q: %w", req.TaskCode, err)
	}

	data, err := s.acceptFiles(ctx, cfg.Files, req.Data)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Record(ctx, Workflow{
		TaskID:   req.TaskID,
		TaskCode: req.TaskCode,
		// Callers speak trade, so the case is keyed by their consignment id.
		CaseID:        req.ConsignmentID,
		CallbackToken: req.CallbackToken,
		Payload:       data,
	}); err != nil {
		return nil, fmt.Errorf("agency: failed to record workflow: %w", err)
	}

	w, err := s.repo.Get(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("agency: failed to read workflow: %w", err)
	}
	// A taskId names one workflow for good, so a repeat is a retry only if it agrees
	// with the recorded row. Checked against the row that won the insert, not before
	// Record, so concurrent requests cannot race past it. It also means cfg below is
	// the row's own config: a retry cannot start a different taskCode's workflow.
	if w.CaseID != req.ConsignmentID || w.TaskCode != req.TaskCode {
		return nil, fmt.Errorf("%w: %q is recorded for consignment %q, task code %q",
			ErrConflict, w.TaskID, w.CaseID, w.TaskCode)
	}
	// Once started (or completed), a repeat inject must not reach the engine: after the
	// workflow completes, the engine would accept the same ID as a brand-new run.
	if w.Status != StatusStarting {
		return w, nil
	}

	if err := s.start(ctx, w, cfg.Workflow); err != nil {
		return nil, err
	}
	if err := s.repo.MarkStarted(ctx, w.TaskID); err != nil {
		return nil, fmt.Errorf("agency: failed to mark workflow started: %w", err)
	}
	w.Status = StatusStarted
	return w, nil
}

// acceptFiles returns data with each file the task config declares at paths
// replaced by what the caller's token for it is stored as (see FileAcceptor). The
// payload is returned as it was sent when no file is declared.
func (s *Service) acceptFiles(ctx context.Context, declared []string, data json.RawMessage) (json.RawMessage, error) {
	if len(declared) == 0 || len(bytes.TrimSpace(data)) == 0 {
		return data, nil
	}
	if s.files == nil {
		return nil, errors.New("agency: the task config declares files, and no file acceptor is configured")
	}
	paths, err := filefields.ParseList(declared)
	if err != nil {
		return nil, fmt.Errorf("agency: %w", err)
	}
	// Numbers stay exact: they are kept as the text they were sent as.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var payload any
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: the payload is not JSON", ErrInvalidFiles)
	}
	caller, _ := authn.FromContext(ctx)
	err = filefields.Replace(payload, paths, func(ref string) (string, error) {
		return s.files.Accept(caller, ref)
	})
	switch {
	case err == nil:
	case errors.Is(err, filefields.ErrShape), errors.Is(err, fileaccess.ErrInvalid), errors.Is(err, fileaccess.ErrNotYours),
		errors.Is(err, fileaccess.ErrScope), errors.Is(err, fileaccess.ErrExpired), errors.Is(err, fileaccess.ErrNoService):
		// The sender's value stays out of the error and the log.
		return nil, ErrInvalidFiles
	default:
		return nil, fmt.Errorf("agency: failed to accept the payload's files: %w", err)
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("agency: failed to encode the payload: %w", err)
	}
	return bytes.TrimRight(out.Bytes(), "\n"), nil
}

// start loads workflowID and starts it with w.TaskID as the instance ID. Variables
// are seeded from the recorded row, not the current request, so a retried start runs
// with the first payload and callback token.
func (s *Service) start(ctx context.Context, w *Workflow, workflowID string) error {
	def, err := workflowdef.Load(ctx, s.artifactRegistry, workflowID)
	if err != nil {
		return fmt.Errorf("agency: failed to load workflow %q: %w", workflowID, err)
	}

	var notification any
	if len(w.Payload) > 0 {
		if err := json.Unmarshal(w.Payload, &notification); err != nil {
			return fmt.Errorf("agency: invalid payload JSON: %w", err)
		}
	}
	// "consignmentId" stays the variable name: workflow artifacts map it by that name.
	vars := map[string]any{
		"taskId":        w.TaskID,
		"taskCode":      w.TaskCode,
		"consignmentId": w.CaseID,
		"notification":  notification,
	}
	// Seeded only when the caller sent one: a workflow that calls back by it then fails
	// on the missing variable at the start, instead of calling back on an empty token.
	if w.CallbackToken != "" {
		vars["callbackToken"] = w.CallbackToken
	}

	if err := s.wm.StartWorkflow(ctx, w.TaskID, def, vars); err != nil {
		return fmt.Errorf("agency: failed to start workflow: %w", err)
	}
	return nil
}

// HandleInject handles POST /api/v1/inject. Response: Workflow, without its payload.
func (s *Service) HandleInject(w http.ResponseWriter, r *http.Request) {
	if s.maxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes)
	}
	var req InjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httputil.Error(w, r, http.StatusRequestEntityTooLarge, errBodyTooLarge)
			return
		}
		httputil.Error(w, r, http.StatusBadRequest, errInvalidBody)
		return
	}
	switch {
	case req.TaskID == "":
		httputil.Error(w, r, http.StatusBadRequest, errTaskIDRequired)
		return
	case req.TaskCode == "":
		httputil.Error(w, r, http.StatusBadRequest, errTaskCodeRequired)
		return
	case req.ConsignmentID == "":
		httputil.Error(w, r, http.StatusBadRequest, errConsignmentIDReq)
		return
	}

	wf, err := s.Inject(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownTaskCode):
			httputil.Error(w, r, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, ErrConflict):
			httputil.Error(w, r, http.StatusConflict, err.Error())
			return
		case errors.Is(err, ErrInvalidFiles):
			httputil.Error(w, r, http.StatusBadRequest, errInvalidFiles)
			return
		}
		httputil.InternalServerError(w, r, errFailedToInjectTask, err, "taskId", req.TaskID)
		return
	}
	// The recorded payload is the sender's; it is not sent back.
	reply := *wf
	reply.Payload = nil
	httputil.JSON(w, http.StatusOK, reply)
}
