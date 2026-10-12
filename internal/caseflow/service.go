// Package caseflow runs the top-level workflows (flows) of a case. A case is a
// long-lived row in cases (in TNSW, a consignment); every flow on it is a finite,
// standalone workflow started through StartFlow and recorded in case_flows. Nothing
// stays open in the engine, so a case can take new flows at any time. Every flow is
// started with the case ID as its root workflow ID, so its tasks are grouped under the
// case.
//
// The case state follows its flows: FINISHED once every flow has completed,
// IN_PROGRESS again when a new one starts. It is a label, never a gate: nothing here
// refuses a flow because its case finished.
//
// This package knows nothing of consignments or agencies; callers supply the case ID
// and the template.
//
// The repository uses database/sql with plain SQL and no ORM: its correctness rests on
// statement order and row locks (see Record and MarkCompleted), which the SQL states
// directly. New repositories follow this style.
package caseflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
	workflow "github.com/OpenNSW/core/workflow"
	"github.com/google/uuid"
)

// ErrUnknownTemplate is returned by StartFlow when no workflow definition is registered
// for the request's template.
var ErrUnknownTemplate = errors.New("unknown flow template")

// ErrConflict is returned by StartFlow when the workflow ID is already recorded for a
// different case or template.
var ErrConflict = errors.New("workflow already recorded with different details")

// ErrInvalidRequest is returned by StartFlow for a request it cannot act on.
var ErrInvalidRequest = errors.New("invalid start flow request")

// workflowIDSep separates the parts of a built workflow ID. A workflow ID never holds
// "/": the admin routes take it as a single path segment.
const workflowIDSep = ":"

// StartRequest is one call to StartFlow.
type StartRequest struct {
	CaseID string
	// TemplateID is the workflow definition the flow runs.
	TemplateID string
	// WorkflowID names the flow. Empty builds "<caseID>:<templateID>:<uuid>"; a caller
	// that can be asked twice for the same flow (a redelivered event) passes a stable ID
	// instead, so the repeat is recognised.
	WorkflowID string
	// Vars seeds the workflow variables. They are recorded with the flow, so a retried
	// start runs with the first request's variables.
	Vars map[string]any
}

// Service starts flows and records their completion.
type Service struct {
	repo             Repository
	artifactRegistry *artifact.Registry
	wm               workflow.Manager
}

// NewService creates a Service. The workflow manager is registered separately (see
// RegisterWorkflowManager), since the manager's completion callback is this Service.
func NewService(repo Repository, artifactRegistry *artifact.Registry) *Service {
	return &Service{repo: repo, artifactRegistry: artifactRegistry}
}

// RegisterWorkflowManager sets the manager flows are started on.
func (s *Service) RegisterWorkflowManager(wm workflow.Manager) error {
	if wm == nil {
		return fmt.Errorf("workflow manager cannot be nil")
	}
	s.wm = wm
	return nil
}

// FlowWorkflowID builds the workflow ID of a flow: "<caseID>:<templateID>:<suffix>".
func FlowWorkflowID(caseID, templateID, suffix string) string {
	return strings.Join([]string{caseID, templateID, suffix}, workflowIDSep)
}

// StartFlow records req's flow and starts it unless it has already started. It is safe
// to retry: a row left STARTING by a failed start is started again, and a start racing
// another for the same workflow ID is deduplicated by the engine, which attaches to the
// running execution instead of starting a second one.
func (s *Service) StartFlow(ctx context.Context, req StartRequest) (*Flow, error) {
	newFlow, def, err := s.prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Record(ctx, newFlow); err != nil {
		return nil, fmt.Errorf("caseflow: failed to record flow: %w", err)
	}

	f, err := s.repo.Get(ctx, newFlow.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("caseflow: failed to read flow: %w", err)
	}
	// A workflow ID names one flow for good, so a repeat is a retry only if it agrees
	// with the recorded row. Checked against the row that won the insert, not before
	// Record, so concurrent requests cannot race past it. It also means def is the row's
	// own definition: a retry cannot start a different template under this ID.
	if f.CaseID != req.CaseID || f.TemplateID != req.TemplateID {
		return nil, fmt.Errorf("%w: %q is recorded for case %q, template %q",
			ErrConflict, f.WorkflowID, f.CaseID, f.TemplateID)
	}
	// Once started (or completed), a repeat must not reach the engine: after the
	// workflow completes, the engine would accept the same ID as a brand-new run.
	if f.Status != StatusStarting {
		return f, nil
	}

	if err := s.start(ctx, f, def); err != nil {
		return nil, err
	}
	if err := s.repo.MarkStarted(ctx, f.WorkflowID); err != nil {
		return nil, fmt.Errorf("caseflow: failed to mark flow started: %w", err)
	}
	f.Status = StatusStarted
	return f, nil
}

// prepare validates req and builds the row to record and the definition to start. It
// touches nothing, so a request it rejects leaves no rows.
func (s *Service) prepare(ctx context.Context, req StartRequest) (Flow, workflow.WorkflowDefinition, error) {
	var def workflow.WorkflowDefinition
	if req.CaseID == "" || req.TemplateID == "" {
		return Flow{}, def, fmt.Errorf("%w: case ID and template are required", ErrInvalidRequest)
	}
	def, err := workflowdef.Load(ctx, s.artifactRegistry, req.TemplateID)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			return Flow{}, def, fmt.Errorf("%w %q", ErrUnknownTemplate, req.TemplateID)
		}
		return Flow{}, def, fmt.Errorf("caseflow: failed to load workflow %q: %w", req.TemplateID, err)
	}

	workflowID := req.WorkflowID
	if workflowID == "" {
		workflowID = FlowWorkflowID(req.CaseID, req.TemplateID, uuid.NewString())
	}
	if strings.Contains(workflowID, "/") {
		return Flow{}, def, fmt.Errorf("%w: workflow ID %q contains \"/\"", ErrInvalidRequest, workflowID)
	}

	var payload json.RawMessage
	if req.Vars != nil {
		if payload, err = json.Marshal(req.Vars); err != nil {
			return Flow{}, def, fmt.Errorf("%w: variables are not JSON: %w", ErrInvalidRequest, err)
		}
	}
	return Flow{WorkflowID: workflowID, CaseID: req.CaseID, TemplateID: req.TemplateID, Payload: payload}, def, nil
}

// start starts f's workflow. Variables are seeded from the recorded row, not the
// current request, so a retried start runs with the first request's variables.
func (s *Service) start(ctx context.Context, f *Flow, def workflow.WorkflowDefinition) error {
	if s.wm == nil {
		return fmt.Errorf("caseflow: no workflow manager registered")
	}
	vars := map[string]any{}
	if len(f.Payload) > 0 && string(f.Payload) != "null" {
		if err := json.Unmarshal(f.Payload, &vars); err != nil {
			return fmt.Errorf("caseflow: recorded payload of %q is not a JSON object: %w", f.WorkflowID, err)
		}
	}
	// Every flow of a case shares the case ID as its root, so the engine files every
	// task it spawns, at any depth, under the case.
	vars[workflow.VarRootWorkflowID] = f.CaseID

	if err := s.wm.StartWorkflow(ctx, f.WorkflowID, def, vars); err != nil {
		return fmt.Errorf("caseflow: failed to start workflow %q: %w", f.WorkflowID, err)
	}
	return nil
}

// EnsureCase creates the cases row for caseID if it does not exist yet, so a domain row
// that references the case (a consignment) can be inserted after it. It runs on its own:
// if the domain insert then fails, the empty case is left behind, which is harmless.
func (s *Service) EnsureCase(ctx context.Context, caseID string) error {
	return s.repo.EnsureCase(ctx, caseID)
}

// Flows returns a case's flows, oldest first.
func (s *Service) Flows(ctx context.Context, caseID string) ([]Flow, error) {
	return s.repo.ListByCase(ctx, caseID)
}

// CompletionHandler is the parent runner's completion callback: it marks the flow
// COMPLETED and, if it was the case's last running flow, the case FINISHED. It runs
// inside a Temporal activity that retries on error, so it is idempotent.
func (s *Service) CompletionHandler(workflowID string, _ map[string]any) error {
	found, err := s.repo.MarkCompleted(context.Background(), workflowID)
	if err != nil {
		return fmt.Errorf("caseflow: failed to record completion of %q: %w", workflowID, err)
	}
	if !found {
		// Every top-level workflow starts through StartFlow, so this is a bug or a
		// workflow from another deployment sharing the Temporal namespace.
		return fmt.Errorf("caseflow: completed workflow %q is not a recorded flow", workflowID)
	}
	return nil
}
