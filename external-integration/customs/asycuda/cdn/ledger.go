package cdn

import (
	"context"
	"fmt"
	"sort"

	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/parkedtask"
)

// Stage is where one container's dispatch note branch stands. The consignment
// opens a branch per container once the declaration is warranted; a branch
// raises one dispatch note, or more when Customs refuses one and the trader
// corrects it.
type Stage string

const (
	// StageNotSubmitted is a branch on the trader's form: not sent yet, or
	// sent and refused, so the trader is correcting it.
	StageNotSubmitted Stage = "NOT_SUBMITTED"
	// StageSubmitting is a branch whose note is on its way to SLC Edge.
	StageSubmitting Stage = "SUBMITTING"
	// StageSubmitted is a branch whose note SLC Edge accepted, waiting for the
	// integration result (§7.2).
	StageSubmitted Stage = "SUBMITTED"
	// StageIntegrated is a branch whose note ASYCUDA registered, waiting for
	// Customs to acknowledge it (§7.3).
	StageIntegrated Stage = "INTEGRATED"
	// StageAcknowledged is a branch that finished with its note acknowledged.
	StageAcknowledged Stage = "ACKNOWLEDGED"
	// StageCancelled is a branch that finished without a note: Customs
	// cancelled it (§7.7), or NSW withdrew it when an amended declaration
	// no longer had its container.
	StageCancelled Stage = "CANCELLED"
)

// Templates the steps of a dispatch note branch run on. userInputTemplateID
// and dispatchTemplateID are the trader's form and the call to SLC Edge;
// integrationWaitTemplateID and acknowledgmentWaitTemplateID are the waits
// resume.go releases.
const (
	userInputTemplateID = "customs-cdn--user-input"
	dispatchTemplateID  = "customs-cdn--cig-dispatch"
)

// branchTemplates are the steps a branch's tasks can be on, so a task on one of
// them belongs to a dispatch note branch.
var branchTemplates = map[string]bool{
	userInputTemplateID:          true,
	dispatchTemplateID:           true,
	integrationWaitTemplateID:    true,
	acknowledgmentWaitTemplateID: true,
}

// Task states the ledger reads a branch's stage from.
const (
	statePendingUser = "PENDING_USER"
	stateCompleted   = "COMPLETED"
)

// Branch is one container's dispatch note branch.
type Branch struct {
	// WorkflowID is the branch's workflow, which its dispatch notes record.
	WorkflowID string
	Stage      Stage
	// Note is the branch's latest dispatch note, nil before SLC Edge accepted
	// the first.
	Note *DispatchNote

	// parked is the step the branch waits on, when it is one NSW may complete
	// to withdraw the branch: the trader's form, or the integration wait.
	parked parkedtask.Task
}

// live reports whether the branch still stands for a container.
func (b Branch) live() bool { return b.Stage != StageCancelled }

// Ledger reports the dispatch note branches of a case: those the split opened,
// and where each stands. It reads them from the tasks the branches run and the
// dispatch notes they raised, so it needs nothing recorded of its own.
type Ledger struct {
	repo  DispatchNoteRepository
	tasks parkedtask.Tasks
}

// NewLedger returns a Ledger over the dispatch notes in repo and core's task
// store.
func NewLedger(repo DispatchNoteRepository, tasks parkedtask.Tasks) Ledger {
	return Ledger{repo: repo, tasks: tasks}
}

// Branches returns the dispatch note branches of caseID, in a fixed order. A
// case whose declaration has not reached the split yet has none.
func (l Ledger) Branches(ctx context.Context, caseID string) ([]Branch, error) {
	records, err := l.tasks.TasksOfCase(ctx, caseID)
	if err != nil {
		return nil, err
	}
	notes, err := l.repo.ListByCase(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("load the dispatch notes of case %s: %w", caseID, err)
	}
	latest := latestNotes(notes)

	type tasksOfBranch struct{ create, ack *store.TaskRecord }
	byWorkflow := map[string]*tasksOfBranch{}
	for i := range records {
		rec := &records[i]
		if !branchTemplates[rec.ActiveTaskTemplateID] {
			continue
		}
		t, ok := byWorkflow[rec.ParentWorkflowID]
		if !ok {
			t = &tasksOfBranch{}
			byWorkflow[rec.ParentWorkflowID] = t
		}
		if rec.ActiveTaskTemplateID == acknowledgmentWaitTemplateID {
			t.ack = rec
		} else {
			t.create = rec
		}
	}

	branches := make([]Branch, 0, len(byWorkflow))
	for workflowID, t := range byWorkflow {
		b := Branch{WorkflowID: workflowID, Note: latest[workflowID]}
		b.Stage, b.parked = stageOf(t.create, t.ack, b.Note)
		branches = append(branches, b)
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].WorkflowID < branches[j].WorkflowID })
	return branches, nil
}

// stageOf reads a branch's stage from its tasks: the CDN task (form, dispatch
// and integration wait) and, once that has finished, the acknowledgment wait.
// It also returns the step the branch is parked on when NSW may withdraw it.
func stageOf(create, ack *store.TaskRecord, note *DispatchNote) (Stage, parkedtask.Task) {
	if ack != nil {
		if ack.State == parkedtask.StateQueuedExternally {
			return StageIntegrated, parkedtask.Task{}
		}
		if note != nil && (note.Status == DispatchNoteStatusCancelled || note.Status == DispatchNoteStatusWithdrawn) {
			return StageCancelled, parkedtask.Task{}
		}
		return StageAcknowledged, parkedtask.Task{}
	}
	if create == nil {
		return StageSubmitting, parkedtask.Task{}
	}
	if withdrawn(create) {
		return StageCancelled, parkedtask.Task{}
	}
	parked := parkedtask.Task{TaskID: create.TaskID, StepID: create.ActiveStepID}
	switch {
	case create.ActiveTaskTemplateID == userInputTemplateID && create.State == statePendingUser:
		return StageNotSubmitted, parked
	case create.ActiveTaskTemplateID == integrationWaitTemplateID && create.State == parkedtask.StateQueuedExternally:
		return StageSubmitted, parked
	case create.State == stateCompleted:
		// Integrated, with the acknowledgment wait not opened yet.
		return StageIntegrated, parkedtask.Task{}
	default:
		return StageSubmitting, parkedtask.Task{}
	}
}

// withdrawn reports whether NSW withdrew the branch's CDN task: the step it
// completed to do so marks the trader's form cancelled.
func withdrawn(create *store.TaskRecord) bool {
	userform, _ := create.Data["userform"].(map[string]any)
	cancelled, _ := userform["cancelled"].(bool)
	return cancelled
}

// latestNotes returns each branch workflow's most recent dispatch note.
func latestNotes(notes []DispatchNote) map[string]*DispatchNote {
	latest := make(map[string]*DispatchNote, len(notes))
	for i := range notes {
		n := &notes[i]
		if cur, ok := latest[n.WorkflowID]; !ok || n.CreatedAt.After(cur.CreatedAt) {
			latest[n.WorkflowID] = n
		}
	}
	return latest
}

// Tally counts branches by stage.
func Tally(branches []Branch) map[Stage]int {
	tally := make(map[Stage]int, len(branches))
	for _, b := range branches {
		tally[b.Stage]++
	}
	return tally
}
