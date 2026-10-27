package cdn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/parkedtask"
)

var amendedDecl = DocumentReference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 1047}

// branchCase is case-1 with a dispatch note branch per entry, each in the
// stage named, and the dispatch notes those stages imply.
type branchCase struct {
	tasks fakeTasks
	repo  *mockRepository
}

func newBranchCase(stages ...Stage) branchCase {
	c := branchCase{tasks: fakeTasks{}, repo: &mockRepository{byEdgeID: map[string]*DispatchNote{}}}
	for i, stage := range stages {
		wf := "wf-branch-" + string(rune('a'+i))
		create := store.TaskRecord{TaskID: "create:" + wf, ActiveStepID: "step:" + wf, ParentWorkflowID: wf, CaseID: "case-1"}
		var ack *store.TaskRecord
		registered := func(status DispatchNoteStatus) {
			c.repo.byEdgeID["edge-"+wf] = &DispatchNote{
				EdgeID: "edge-" + wf, Status: status, CaseID: "case-1", WorkflowID: wf, CreatedAt: time.Unix(int64(i), 0),
				CDNOffice: "CBEX1", CDNYear: "2026", CDNSerial: "C", CDNNumber: 100 + i,
			}
		}
		switch stage {
		case StageNotSubmitted:
			create.ActiveTaskTemplateID, create.State = userInputTemplateID, statePendingUser
		case StageSubmitting:
			create.ActiveTaskTemplateID, create.State = dispatchTemplateID, "IN_PROGRESS"
		case StageSubmitted:
			create.ActiveTaskTemplateID, create.State = integrationWaitTemplateID, parkedtask.StateQueuedExternally
			c.repo.byEdgeID["edge-"+wf] = &DispatchNote{EdgeID: "edge-" + wf, Status: DispatchNoteStatusSubmitted, CaseID: "case-1", WorkflowID: wf}
		case StageIntegrated:
			create.ActiveTaskTemplateID, create.State = integrationWaitTemplateID, stateCompleted
			ack = &store.TaskRecord{TaskID: "ack:" + wf, ParentWorkflowID: wf, CaseID: "case-1",
				ActiveTaskTemplateID: acknowledgmentWaitTemplateID, State: parkedtask.StateQueuedExternally}
			registered(DispatchNoteStatusIntegrated)
		case StageAcknowledged, StageCancelled:
			create.ActiveTaskTemplateID, create.State = integrationWaitTemplateID, stateCompleted
			ack = &store.TaskRecord{TaskID: "ack:" + wf, ParentWorkflowID: wf, CaseID: "case-1",
				ActiveTaskTemplateID: acknowledgmentWaitTemplateID, State: stateCompleted}
			if stage == StageAcknowledged {
				registered(DispatchNoteStatusAcknowledged)
			} else {
				registered(DispatchNoteStatusCancelled)
			}
		}
		c.tasks["case-1"] = append(c.tasks["case-1"], create)
		if ack != nil {
			c.tasks["case-1"] = append(c.tasks["case-1"], *ack)
		}
	}
	// A task outside the CDN branches is not one.
	c.tasks["case-1"] = append(c.tasks["case-1"], store.TaskRecord{TaskID: "boat-note", ParentWorkflowID: "wf-customs",
		CaseID: "case-1", ActiveTaskTemplateID: "customs-boat-note--user-input", State: statePendingUser})
	return c
}

func TestLedger_ReadsEachBranchStage(t *testing.T) {
	c := newBranchCase(StageNotSubmitted, StageSubmitting, StageSubmitted, StageIntegrated, StageAcknowledged, StageCancelled)

	branches, err := NewLedger(c.repo, c.tasks).Branches(context.Background(), "case-1")
	require.NoError(t, err)

	var stages []Stage
	for _, b := range branches {
		stages = append(stages, b.Stage)
	}
	assert.Equal(t, []Stage{StageNotSubmitted, StageSubmitting, StageSubmitted, StageIntegrated, StageAcknowledged, StageCancelled}, stages)
	assert.Equal(t, map[Stage]int{StageNotSubmitted: 1, StageSubmitting: 1, StageSubmitted: 1, StageIntegrated: 1, StageAcknowledged: 1, StageCancelled: 1}, Tally(branches))
	assert.Equal(t, "edge-wf-branch-c", branches[2].Note.EdgeID, "a branch carries the note it raised")
	assert.Nil(t, branches[0].Note, "a branch not yet sent has no note")
}

// A branch NSW withdrew is cancelled, whichever step it was withdrawn from.
func TestLedger_ReadsAWithdrawnBranchAsCancelled(t *testing.T) {
	c := newBranchCase(StageNotSubmitted)
	c.tasks["case-1"][0].State = stateCompleted
	c.tasks["case-1"][0].Data = map[string]any{"userform": map[string]any{"cancelled": true}}

	branches, err := NewLedger(c.repo, c.tasks).Branches(context.Background(), "case-1")
	require.NoError(t, err)
	require.Len(t, branches, 1)
	assert.Equal(t, StageCancelled, branches[0].Stage)
}

func TestLedger_ReportsATaskStoreFailure(t *testing.T) {
	_, err := NewLedger(&mockRepository{}, failingTasks{}).Branches(context.Background(), "case-1")
	assert.ErrorIs(t, err, errTaskStore)
}

// amendedTo is an amended declaration with n containers, under the declarant
// and exporter the retrieve has to name.
func amendedTo(n int) map[string]any {
	return map[string]any{
		"baseGeneralSegment": map[string]any{"declarantCode": "D0012345", "exporter": map[string]any{"id": "C003"}},
		"generalSegment":     map[string]any{"numberOfContainers": float64(n)},
	}
}

// retrieveAnswer is SLC Edge's §6.7 answer: the declaration with n containers
// and the dispatch notes registered against it.
type retrieveAnswer struct {
	containers int
	cdnRefs    []map[string]any
	err        error
	calls      []document.DeclarationRetrieve
}

func (r *retrieveAnswer) retrieve(_ context.Context, req document.DeclarationRetrieve) (map[string]any, error) {
	r.calls = append(r.calls, req)
	if r.err != nil {
		return nil, r.err
	}
	refs := make([]any, len(r.cdnRefs))
	for i, ref := range r.cdnRefs {
		refs[i] = ref
	}
	return map[string]any{
		"eventType": "CUSDEC_RETRIEVED",
		"payload": map[string]any{
			"cusdec":  amendedTo(r.containers),
			"cdnRefs": refs,
		},
	}, nil
}

func registeredRefs(n int, status string) []map[string]any {
	refs := make([]map[string]any, n)
	for i := range refs {
		refs[i] = map[string]any{"office": "CBEX1", "year": "2026", "serial": "C", "number": float64(100 + i), "status": status}
	}
	return refs
}

// Two of four containers are dropped. NSW and ASYCUDA agree that one note is
// registered, so the branch still on the trader's form goes first, then the
// one waiting to be integrated; the registered one is left to Customs.
func TestReconcile_WithdrawsTheBranchesOfDroppedContainers(t *testing.T) {
	c := newBranchCase(StageSubmitted, StageNotSubmitted, StageIntegrated, StageNotSubmitted)
	answer := &retrieveAnswer{containers: 2, cdnRefs: registeredRefs(1, "Registered")}
	tasks := &resumeRecorder{}

	err := NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(2))
	require.NoError(t, err)

	require.Len(t, answer.calls, 1)
	assert.Equal(t, document.DeclarationRetrieve{CusdecRef: amendedDecl, DeclarantCode: "D0012345", CompanyCode: "C003"}, answer.calls[0],
		"the retrieve names the declaration under the codes the amendment carries")

	require.Len(t, tasks.completed, 2, "two containers dropped, two branches withdrawn")
	assert.Equal(t, "cancel", tasks.completed["create:wf-branch-b"]["__command"], "the first unsent branch is cancelled from its form")
	assert.Equal(t, "cancel", tasks.completed["create:wf-branch-d"]["__command"])
	assert.Equal(t, true, tasks.completed["create:wf-branch-b"]["cancelled"])
	assert.Contains(t, tasks.completed["create:wf-branch-b"]["cancellation_reason"], "CBEX1/2026/E/1047 to 2 containers")
	assert.NotContains(t, tasks.completed, "create:wf-branch-a", "a sent branch is kept while unsent ones cover the excess")
	assert.Nil(t, c.repo.updated, "no note is touched")
}

// When the unsent branches don't cover the excess, the branch waiting to be
// integrated is withdrawn too: its wait is released as not integrated and its
// note marked withdrawn.
func TestReconcile_WithdrawsASentBranchWhenNeeded(t *testing.T) {
	c := newBranchCase(StageSubmitted, StageNotSubmitted, StageIntegrated)
	answer := &retrieveAnswer{containers: 1, cdnRefs: registeredRefs(1, "Registered")}
	tasks := &resumeRecorder{}

	require.NoError(t, NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(1)))

	require.Len(t, tasks.completed, 2)
	sent := tasks.completed["create:wf-branch-a"]
	assert.Equal(t, "submit", sent["__command"])
	assert.Equal(t, false, sent["integrated"])
	assert.Equal(t, true, sent["cancelled"])
	assert.Equal(t, "step:wf-branch-a", tasks.steps["create:wf-branch-a"], "only the step the branch is parked on is completed")
	require.NotNil(t, c.repo.updated, "the withdrawn branch's note is recorded")
	assert.Equal(t, "edge-wf-branch-a", c.repo.updated.EdgeID)
	assert.Equal(t, DispatchNoteStatusWithdrawn, c.repo.updated.Status)
}

// NSW holds one registered note and ASYCUDA two: a result is on its way, and a
// branch about to be integrated could be withdrawn, so nothing is.
func TestReconcile_WithdrawsNothingWhenTheRegisteredCountsDiffer(t *testing.T) {
	c := newBranchCase(StageSubmitted, StageNotSubmitted, StageIntegrated)
	answer := &retrieveAnswer{containers: 1, cdnRefs: registeredRefs(2, "Registered")}
	tasks := &resumeRecorder{}

	require.NoError(t, NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(1)))

	assert.Empty(t, tasks.completed)
	assert.Nil(t, c.repo.updated)
}

// A note Customs cancelled counts on neither side.
func TestReconcile_LeavesCancelledNotesOutOfTheCounts(t *testing.T) {
	c := newBranchCase(StageNotSubmitted, StageIntegrated, StageCancelled)
	refs := append(registeredRefs(1, "Registered"), map[string]any{"office": "CBEX1", "year": "2026", "serial": "C", "number": float64(102), "status": "Cancelled"})
	answer := &retrieveAnswer{containers: 1, cdnRefs: refs}
	tasks := &resumeRecorder{}

	require.NoError(t, NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(1)))

	require.Len(t, tasks.completed, 1)
	assert.Contains(t, tasks.completed, "create:wf-branch-a")
}

// No container dropped, or the split not reached yet: nothing is retrieved or
// withdrawn.
func TestReconcile_DoesNothingWithoutDroppedContainers(t *testing.T) {
	for name, c := range map[string]branchCase{
		"as many branches as containers": newBranchCase(StageNotSubmitted, StageIntegrated),
		"split not reached":              {tasks: fakeTasks{}, repo: &mockRepository{byEdgeID: map[string]*DispatchNote{}}},
		"a dropped branch already gone":  newBranchCase(StageNotSubmitted, StageIntegrated, StageCancelled),
	} {
		t.Run(name, func(t *testing.T) {
			answer := &retrieveAnswer{containers: 2}
			tasks := &resumeRecorder{}
			require.NoError(t, NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
				ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(2)))
			assert.Empty(t, answer.calls)
			assert.Empty(t, tasks.completed)
		})
	}
}

// The declaration as ASYCUDA now holds it decides the count, over the
// amendment it was read from.
func TestReconcile_UsesTheRetrievedContainerCount(t *testing.T) {
	c := newBranchCase(StageNotSubmitted, StageNotSubmitted, StageNotSubmitted)
	answer := &retrieveAnswer{containers: 2}
	tasks := &resumeRecorder{}

	require.NoError(t, NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(1)))

	assert.Len(t, tasks.completed, 1)
}

// More notes registered than containers remain: every branch NSW may withdraw
// goes, and the rest is left to Customs.
func TestReconcile_WithdrawsOnlyWhatNSWMay(t *testing.T) {
	c := newBranchCase(StageIntegrated, StageIntegrated, StageNotSubmitted)
	answer := &retrieveAnswer{containers: 1, cdnRefs: registeredRefs(2, "Registered")}
	tasks := &resumeRecorder{}

	require.NoError(t, NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(1)))

	require.Len(t, tasks.completed, 1)
	assert.Contains(t, tasks.completed, "create:wf-branch-c")
}

// A retrieve that fails is returned, so SLC Edge redelivers the amendment.
func TestReconcile_ReturnsAFailedRetrieve(t *testing.T) {
	c := newBranchCase(StageNotSubmitted, StageNotSubmitted)
	answer := &retrieveAnswer{err: errors.New("slc edge: 503")}
	tasks := &resumeRecorder{}

	err := NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, amendedTo(1))
	require.Error(t, err)
	assert.Empty(t, tasks.completed)
}

// An amended declaration with no usable container count changes nothing.
func TestReconcile_IgnoresAnAmendmentWithoutAContainerCount(t *testing.T) {
	c := newBranchCase(StageNotSubmitted, StageNotSubmitted)
	answer := &retrieveAnswer{containers: 1}
	tasks := &resumeRecorder{}

	require.NoError(t, NewReconciler(c.repo, c.tasks, tasks, answer.retrieve).
		ReconcileContainers(context.Background(), "case-1", amendedDecl, map[string]any{"generalSegment": map[string]any{}}))
	assert.Empty(t, answer.calls)
	assert.Empty(t, tasks.completed)
}
