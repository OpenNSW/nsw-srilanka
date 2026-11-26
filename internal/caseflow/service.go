// Package caseflow runs the top-level workflows (flows) of a case. A case is a
// long-lived row in cases (in TNSW, a consignment); every flow on it is a finite,
// standalone workflow started through StartFlow and recorded in case_flows. Nothing
// stays open in the engine, so a case can take new flows at any time. Every flow is
// started with the case ID as its engine case (workflow.VarCaseID), so its tasks are
// grouped under the case, and with the artifact manifest version current when it was
// recorded (workflow.VarManifestVersion), so a deploy never changes its templates.
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
	"log/slog"
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
	// PriorWorkflowID, if set, is an ID an earlier version of the caller named the same
	// flow by. When no flow is recorded under WorkflowID, a flow recorded under this one
	// for the same case and template is the flow asked for, and is resumed instead of
	// starting another. One recorded for another case or template is not, and a new flow
	// starts under WorkflowID.
	PriorWorkflowID string
	// Vars seeds the workflow variables. They are recorded with the flow, so a retried
	// start runs with the first request's variables.
	Vars map[string]any
	// Owner, if set, names the owner a new flow records for its case, in the same
	// transaction as the flow, so a recorded flow always has its owner. A retry of a
	// recorded flow does not call it: its owner committed with it.
	Owner OwnerOf
	// Case, if set, is written to the case in the same transaction as the flow. A retry of
	// a recorded flow writes nothing: its first write committed with it.
	Case *CaseInfo
}

// Owner is an OU that holds roles on a flow's case (case_parties).
type Owner struct {
	OUHandle string
	Roles    []string
}

// OwnerOf returns the owner of the case of a new flow that runs def. reg holds the
// artifacts of the manifest version the flow runs on, so the owner's roles can follow the
// templates the flow runs.
type OwnerOf func(ctx context.Context, reg *artifact.Registry, def workflow.WorkflowDefinition) (*Owner, error)

// CaseInfo is what a new flow writes to its case. Attributes are merged into
// cases.attributes by top-level key, so a later flow replaces a key's whole value and keeps
// the keys it does not send. A Name that is not empty replaces cases.name.
type CaseInfo struct {
	Attributes map[string]any
	Name       string
}

// Service starts flows and records their completion.
type Service struct {
	repo      Repository
	artifacts *artifact.Set
	wm        workflow.Manager
}

// NewService creates a Service. The workflow manager is registered separately (see
// RegisterWorkflowManager), since the manager's completion callback is this Service.
func NewService(repo Repository, artifacts *artifact.Set) *Service {
	return &Service{repo: repo, artifacts: artifacts}
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
//
// The flow runs on the artifact manifest version current when its row was first
// recorded; a retry after a deploy starts it on that version, not the new current one.
func (s *Service) StartFlow(ctx context.Context, req StartRequest) (*Flow, error) {
	newFlow, err := newFlowRow(req)
	if err != nil {
		return nil, err
	}

	// A retry of a recorded flow starts on the manifest version it was recorded with,
	// so it is not checked against the current one, which may no longer hold its
	// template. Only a caller-chosen workflow ID can name a recorded flow.
	if req.WorkflowID != "" {
		f, err := s.repo.Get(ctx, newFlow.WorkflowID)
		switch {
		case err == nil:
			return s.resume(ctx, req, f, nil)
		case !errors.Is(err, ErrFlowNotFound):
			return nil, fmt.Errorf("caseflow: failed to read flow: %w", err)
		}
		f, err = s.priorFlow(ctx, req)
		if err != nil {
			return nil, err
		}
		if f != nil {
			slog.InfoContext(ctx, "caseflow: resuming the flow recorded under its prior ID",
				"workflowId", newFlow.WorkflowID, "priorWorkflowId", f.WorkflowID, "caseId", req.CaseID)
			return s.resume(ctx, req, f, nil)
		}
	}

	// A new flow runs on the current version. Its template is checked before anything is
	// written, so a request for an unknown template leaves no rows.
	newFlow.ManifestVersion = s.artifacts.Current()
	def, err := s.definition(ctx, newFlow.ManifestVersion, req.TemplateID)
	if err != nil {
		return nil, err
	}
	owner, err := s.owner(ctx, newFlow.ManifestVersion, def, req.Owner)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Record(ctx, newFlow, owner, req.Case); err != nil {
		return nil, fmt.Errorf("caseflow: failed to record flow: %w", err)
	}
	f, err := s.repo.Get(ctx, newFlow.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("caseflow: failed to read flow: %w", err)
	}
	// A concurrent request may have recorded the row first, on another replica's version.
	if f.ManifestVersion != newFlow.ManifestVersion {
		return s.resume(ctx, req, f, nil)
	}
	return s.resume(ctx, req, f, &def)
}

// priorFlow returns the flow recorded under req.PriorWorkflowID if it is for req's case and
// template, and nil if there is none or it is for another.
func (s *Service) priorFlow(ctx context.Context, req StartRequest) (*Flow, error) {
	if req.PriorWorkflowID == "" || req.PriorWorkflowID == req.WorkflowID {
		return nil, nil
	}
	f, err := s.repo.Get(ctx, req.PriorWorkflowID)
	switch {
	case errors.Is(err, ErrFlowNotFound):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("caseflow: failed to read flow: %w", err)
	case f.CaseID != req.CaseID || f.TemplateID != req.TemplateID:
		return nil, nil
	}
	return f, nil
}

// resume starts the recorded flow f unless it has already started. def is f's definition
// if the caller has loaded it; nil loads it from f's manifest version.
func (s *Service) resume(ctx context.Context, req StartRequest, f *Flow, def *workflow.WorkflowDefinition) (*Flow, error) {
	// A workflow ID names one flow for good, so a repeat is a retry only if it agrees
	// with the recorded row. Checked against the row that won the insert, so concurrent
	// requests cannot race past it. It also means a retry cannot start a different
	// template under this ID.
	if f.CaseID != req.CaseID || f.TemplateID != req.TemplateID {
		return nil, fmt.Errorf("%w: %q is recorded for case %q, template %q",
			ErrConflict, f.WorkflowID, f.CaseID, f.TemplateID)
	}
	// Once started (or completed), a repeat must not reach the engine: after the
	// workflow completes, the engine would accept the same ID as a brand-new run.
	if f.Status != StatusStarting {
		return f, nil
	}
	if def == nil {
		loaded, err := s.definition(ctx, f.ManifestVersion, f.TemplateID)
		if err != nil {
			return nil, err
		}
		def = &loaded
	}

	if err := s.start(ctx, f, *def); err != nil {
		return nil, err
	}
	if err := s.repo.MarkStarted(ctx, f.WorkflowID); err != nil {
		return nil, fmt.Errorf("caseflow: failed to mark flow started: %w", err)
	}
	f.Status = StatusStarted
	return f, nil
}

// newFlowRow validates req and builds the row to record, without a manifest version. It
// touches nothing, so a request it rejects leaves no rows.
func newFlowRow(req StartRequest) (Flow, error) {
	if req.CaseID == "" || req.TemplateID == "" {
		return Flow{}, fmt.Errorf("%w: case ID and template are required", ErrInvalidRequest)
	}
	workflowID := req.WorkflowID
	if workflowID == "" {
		workflowID = FlowWorkflowID(req.CaseID, req.TemplateID, uuid.NewString())
	}
	if strings.Contains(workflowID, "/") {
		return Flow{}, fmt.Errorf("%w: workflow ID %q contains \"/\"", ErrInvalidRequest, workflowID)
	}

	var payload json.RawMessage
	if req.Vars != nil {
		var err error
		if payload, err = json.Marshal(req.Vars); err != nil {
			return Flow{}, fmt.Errorf("%w: variables are not JSON: %w", ErrInvalidRequest, err)
		}
	}
	return Flow{WorkflowID: workflowID, CaseID: req.CaseID, TemplateID: req.TemplateID, Payload: payload}, nil
}

// definition loads templateID's workflow definition from manifest version.
func (s *Service) definition(ctx context.Context, version int, templateID string) (workflow.WorkflowDefinition, error) {
	reg, err := s.artifacts.Registry(ctx, version)
	if err != nil {
		return workflow.WorkflowDefinition{}, fmt.Errorf("caseflow: %w", err)
	}
	def, err := workflowdef.Load(ctx, reg, templateID)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			return workflow.WorkflowDefinition{}, fmt.Errorf("%w %q", ErrUnknownTemplate, templateID)
		}
		return workflow.WorkflowDefinition{}, fmt.Errorf("caseflow: failed to load workflow %q: %w", templateID, err)
	}
	return def, nil
}

// owner returns the owner ownerOf names for a new flow that runs def on manifest version,
// or nil when ownerOf is nil.
func (s *Service) owner(ctx context.Context, version int, def workflow.WorkflowDefinition, ownerOf OwnerOf) (*Owner, error) {
	if ownerOf == nil {
		return nil, nil
	}
	reg, err := s.artifacts.Registry(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("caseflow: %w", err)
	}
	owner, err := ownerOf(ctx, reg, def)
	if err != nil {
		return nil, fmt.Errorf("caseflow: %w", err)
	}
	return owner, nil
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
	vars[workflow.VarCaseID] = f.CaseID
	// Every template the flow reaches, at any depth, resolves through its recorded version.
	vars[workflow.VarManifestVersion] = f.ManifestVersion

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
