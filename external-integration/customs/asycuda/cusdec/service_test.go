package cusdec

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockCusdecRepository struct {
	declsByEdgeID map[string]*CusdecDeclaration
	createCalled  bool
	updateCalled  bool
	createdDecl   *CusdecDeclaration
	updatedDecl   *CusdecDeclaration
}

func (m *mockCusdecRepository) GetByEdgeID(ctx context.Context, edgeID string) (*CusdecDeclaration, error) {
	return m.declsByEdgeID[edgeID], nil
}

func (m *mockCusdecRepository) GetByCusdecRef(ctx context.Context, ref DocumentReference) (*CusdecDeclaration, error) {
	for _, d := range m.declsByEdgeID {
		if d.CusdecOffice == ref.Office && d.CusdecYear == ref.Year && d.CusdecSerial == ref.Serial && d.CusdecNumber == ref.Number {
			return d, nil
		}
	}
	return nil, nil
}

func (m *mockCusdecRepository) Create(ctx context.Context, decl *CusdecDeclaration) error {
	m.createCalled = true
	m.createdDecl = decl
	return nil
}

func (m *mockCusdecRepository) Update(ctx context.Context, decl *CusdecDeclaration) error {
	m.updateCalled = true
	m.updatedDecl = decl
	return nil
}

func (m *mockCusdecRepository) RecordSubmitted(ctx context.Context, edgeID, caseID, workflowID string) error {
	m.declsByEdgeID[edgeID] = &CusdecDeclaration{EdgeID: edgeID, Status: CusdecStatusSubmitted, CaseID: caseID, WorkflowID: workflowID}
	return nil
}

type mockTaskCompleter struct {
	mock.Mock
}

func (m *mockTaskCompleter) CompleteTaskStep(ctx context.Context, taskID, stepID string, payload map[string]any) error {
	args := m.Called(ctx, taskID, stepID, payload)
	return args.Error(0)
}

// The case and workflow the tests' declaration was dispatched from.
const (
	testCaseID     = "case-1"
	testWorkflowID = "parent-wf-123"
)

// fakeTasks stands in for core's task store: the tasks of each case.
type fakeTasks map[string][]store.TaskRecord

func (f fakeTasks) TasksOfCase(_ context.Context, caseID string) ([]store.TaskRecord, error) {
	return f[caseID], nil
}

// failingTasks is a task store whose load fails.
type failingTasks struct{}

func (failingTasks) TasksOfCase(context.Context, string) ([]store.TaskRecord, error) {
	return nil, errTaskStore
}

var errTaskStore = errors.New("task store: connection refused")

// parkedOn returns the tests' case with one task of the declaration's workflow
// parked on templateID, beside a task of another workflow parked on the same
// template that must not be resumed.
func parkedOn(templateID, taskID, stepID string) fakeTasks {
	return fakeTasks{testCaseID: {
		{TaskID: "other-task", ActiveStepID: "other-step", ParentWorkflowID: "other-wf", CaseID: testCaseID,
			ActiveTaskTemplateID: templateID, State: "QUEUED_EXTERNALLY"},
		{TaskID: taskID, ActiveStepID: stepID, ParentWorkflowID: testWorkflowID, CaseID: testCaseID,
			ActiveTaskTemplateID: templateID, State: "QUEUED_EXTERNALLY"},
	}}
}

// parkedInBranch returns the tests' case with one task parked on templateID in
// a branch of a parallel split: a child workflow of its own, so its parent is
// never the workflow the declaration was dispatched from.
func parkedInBranch(templateID, taskID, stepID string) fakeTasks {
	return fakeTasks{testCaseID: {
		{TaskID: "dispatch-task", ActiveStepID: "dispatch-step", ParentWorkflowID: testWorkflowID, CaseID: testCaseID,
			ActiveTaskTemplateID: reviewTemplateID, State: "COMPLETED"},
		{TaskID: taskID, ActiveStepID: stepID, ParentWorkflowID: "split-branch-wf", CaseID: testCaseID,
			ActiveTaskTemplateID: templateID, State: "QUEUED_EXTERNALLY"},
	}}
}

// submitted is the declaration as its dispatch left it: SLC Edge accepted it as
// edgeID from the tests' workflow.
func submitted(edgeID string) *CusdecDeclaration {
	return &CusdecDeclaration{ID: "decl-" + edgeID, EdgeID: edgeID, Status: CusdecStatusSubmitted, CaseID: testCaseID, WorkflowID: testWorkflowID}
}

func TestProcessCusdecIntegrationResult_Success(t *testing.T) {
	ctx := context.Background()
	payable1244 := 1244.0

	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": submitted("edge-123")},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, parkedOn(reviewTemplateID, "task-abc", "step-task-abc"), completer)

	req := CusdecIntegrationResultRequest{
		EdgeID:     "edge-123",
		Integrated: true,
		Event:      "INTEGRATION_RESULT",
		ProcessAt:  time.Now(),
		Payload: cusdecResultPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
			// §6.2 returns what is still due alongside the reference; that is
			// what the trader is asked to settle on the payment step.
			AmountPayable: &payable1244,
		},
	}

	expectedPayload := map[string]any{
		"__command":      "submit",
		"review_outcome": "approve",
		"cusdec_number":  "COL/2026/C/9876",
		"amount_to_pay":  float64(1244),
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-abc", "step-task-abc", expectedPayload).Return(nil)

	err := service.ProcessIntegrationResult(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusIntegrated, repo.updatedDecl.Status)
	assert.Equal(t, "COL", repo.updatedDecl.CusdecOffice)
	assert.Equal(t, 9876, repo.updatedDecl.CusdecNumber)

	completer.AssertExpectations(t)
}

// A v1.9 result hands the review step what is still due, and the assessment
// and the amount already paid beside it.
func TestProcessCusdecIntegrationResult_V19Assessment(t *testing.T) {
	ctx := context.Background()

	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-v19": submitted("edge-v19")}}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, parkedOn(reviewTemplateID, "task-v19", "step-task-v19"), completer)

	assessed, paid, payable := 1350.0, 350.0, 1000.0
	req := CusdecIntegrationResultRequest{
		EdgeID:     "edge-v19",
		Integrated: true,
		Event:      "CUSDEC_INTEGRATED",
		ProcessAt:  time.Now(),
		Payload: cusdecResultPayload{
			CusdecRef:           DocumentReference{Year: "2026", Office: "CBEX1", Serial: "E", Number: 59},
			TotalAssessedAmount: &assessed,
			AmountPaid:          &paid,
			AmountPayable:       &payable,
		},
	}
	require.NoError(t, json.Unmarshal([]byte(`{
		"globalDuties": [{"typeCode": "EPF", "taxBaseAmount": 550, "taxRateNumeric": 2.0, "taxAssessedAmount": 1100, "paymentMethodCode": "1"}],
		"itemDutiesList": [{"itemSequenceNumeric": 1, "dutyTaxFees": [{"typeCode": "CED", "taxBaseAmount": 125, "taxRateNumeric": 0.0, "taxAssessedAmount": 250, "paymentMethodCode": "1"}]}]
	}`), &req.Payload.Duties))

	completer.On("CompleteTaskStep", mock.Anything, "task-v19", "step-task-v19", map[string]any{
		"__command":             "submit",
		"review_outcome":        "approve",
		"cusdec_number":         "CBEX1/2026/E/59",
		"amount_to_pay":         1000.0,
		"total_assessed_amount": 1350.0,
		"amount_paid":           350.0,
		// The breakdown, in the spec's own field names, for the review panel.
		"duties": map[string]any{
			"globalDuties": []any{
				map[string]any{"typeCode": "EPF", "taxBaseAmount": 550.0, "taxRateNumeric": 2.0, "taxAssessedAmount": 1100.0, "paymentMethodCode": "1"},
			},
			"itemDutiesList": []any{
				map[string]any{"itemSequenceNumeric": 1.0, "dutyTaxFees": []any{
					map[string]any{"typeCode": "CED", "taxBaseAmount": 125.0, "taxRateNumeric": 0.0, "taxAssessedAmount": 250.0, "paymentMethodCode": "1"},
				}},
			},
		},
	}).Return(nil)

	require.NoError(t, service.ProcessIntegrationResult(ctx, req))
	completer.AssertExpectations(t)
}

func TestProcessEvent_PaymentSuccess(t *testing.T) {
	ctx := context.Background()

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusIntegrated,
		CaseID:       testCaseID,
		WorkflowID:   testWorkflowID,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, parkedInBranch("customs-wait-payment", "task-payment-123", "step-task-payment-123"), completer)

	req := CusdecEventRequest{
		Event:     "PAYMENT_CONFIRMED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
			AmountPaid:    2035.00,
			Currency:      "LKR",
			BankReference: "84004328",
		},
	}

	expectedPayload := map[string]any{
		"__command":      "submit",
		"payment_status": "PAID",
		// §6.5.1 fields reach the trader's receipt, so they must survive the
		// hop from callback to task rather than being parsed and dropped.
		"amount_paid":    2035.00,
		"currency":       "LKR",
		"bank_reference": "84004328",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-payment-123", "step-task-payment-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusPaid, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
}

func TestProcessEvent_DuplicateEventIsAcknowledgedWithoutAction(t *testing.T) {
	ctx := context.Background()

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusPaid,
		CaseID:       testCaseID,
		WorkflowID:   testWorkflowID,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, fakeTasks{}, completer)

	req := CusdecEventRequest{
		Event:     "PAYMENT_CONFIRMED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
		},
	}

	err := service.ProcessEvent(ctx, req)

	// The declaration is already PAID, so this PAYMENT_CONFIRMED has been
	// applied before: acknowledged without touching the declaration or its
	// workflow, and without so much as a lookup.
	require.ErrorIs(t, err, ErrDuplicateEvent)

	assert.False(t, repo.updateCalled)
	completer.AssertExpectations(t)
}

func TestProcessEvent_WarrantingSuccess(t *testing.T) {
	ctx := context.Background()

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusPaid,
		CaseID:       testCaseID,
		WorkflowID:   testWorkflowID,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, parkedInBranch("customs-wait-warranting", "task-warranting-123", "step-task-warranting-123"), completer)

	req := CusdecEventRequest{
		Event:     "WARRANTING_COMPLETED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
		},
	}

	expectedPayload := map[string]any{
		"__command":         "submit",
		"warranting_status": "WARRANTED",
		"release_order_no":  "",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-warranting-123", "step-task-warranting-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusWarranted, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
}

func TestProcessEvent_ReleaseSuccess(t *testing.T) {
	ctx := context.Background()

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusWarranted,
		CaseID:       testCaseID,
		WorkflowID:   testWorkflowID,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, parkedInBranch("customs-wait-export-release", "task-release-123", "step-task-release-123"), completer)

	req := CusdecEventRequest{
		Event:     "EXPORT_RELEASED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
			VesselName:    "EVER GIVEN",
			VoyageNo:      "023W 08/01/2025",
			PortOfLoading: "LKCMB",
		},
	}

	expectedPayload := map[string]any{
		"__command":       "submit",
		"release_status":  "RELEASED",
		"vessel_name":     "EVER GIVEN",
		"voyage_no":       "023W 08/01/2025",
		"port_of_loading": "LKCMB",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-release-123", "step-task-release-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusReleased, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
}

// A declaration dispatched before its workflow was recorded cannot be routed:
// the integration result is refused as not found, so SLC Edge retries it, rather
// than resuming a task of some other workflow.
func TestProcessCusdecIntegrationResult_UnlinkedDeclaration(t *testing.T) {
	decl := submitted("edge-unlinked")
	decl.CaseID, decl.WorkflowID = "", ""
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-unlinked": decl}}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, parkedOn(reviewTemplateID, "task-abc", "step-task-abc"), completer)

	err := service.ProcessIntegrationResult(context.Background(), CusdecIntegrationResultRequest{
		EdgeID: "edge-unlinked", Integrated: true,
		Payload: cusdecResultPayload{CusdecRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 1}},
	})

	require.ErrorIs(t, err, ErrWorkflowNotFoundByEdgeID)
	completer.AssertNotCalled(t, "CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The review task has not parked yet: the result is answered with an error that
// is not "not found", and nothing is recorded, so SLC Edge retries it (§2)
// until the task is there.
func TestProcessCusdecIntegrationResult_ReviewNotParkedYet(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": submitted("edge-123")}}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, fakeTasks{}, completer)

	err := service.ProcessIntegrationResult(context.Background(), CusdecIntegrationResultRequest{
		EdgeID: "edge-123", Integrated: true,
		Payload: cusdecResultPayload{CusdecRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 1}},
	})

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrWorkflowNotFoundByEdgeID)
	assert.False(t, repo.updateCalled, "the result must not be recorded while the step is unresumed")
	assert.Equal(t, CusdecStatusSubmitted, repo.declsByEdgeID["edge-123"].Status)
	completer.AssertNotCalled(t, "CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// SLC Edge's retry of a result that arrived before the review task parked
// resumes the task once it has: the first attempt recorded nothing, so the
// retry is not taken for a duplicate.
func TestProcessCusdecIntegrationResult_RetryAfterNotParkedYetCompletes(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": submitted("edge-123")}}
	completer := &mockTaskCompleter{}
	req := CusdecIntegrationResultRequest{
		EdgeID: "edge-123", Integrated: true,
		Payload: cusdecResultPayload{CusdecRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 1}},
	}

	err := NewWebhookService(repo, fakeTasks{}, completer).ProcessIntegrationResult(context.Background(), req)
	require.Error(t, err)

	completer.On("CompleteTaskStep", mock.Anything, "task-abc", "step-task-abc", mock.Anything).Return(nil)
	err = NewWebhookService(repo, parkedOn(reviewTemplateID, "task-abc", "step-task-abc"), completer).
		ProcessIntegrationResult(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, CusdecStatusIntegrated, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
}

// A review step that could not be completed leaves the declaration SUBMITTED,
// so SLC Edge's retry tries the step again rather than being acknowledged.
func TestProcessCusdecIntegrationResult_CompletionFailureLeavesTheStatus(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": submitted("edge-123")}}
	completer := &mockTaskCompleter{}
	completer.On("CompleteTaskStep", mock.Anything, "task-abc", "step-task-abc", mock.Anything).Return(errors.New("temporal: unavailable"))
	service := NewWebhookService(repo, parkedOn(reviewTemplateID, "task-abc", "step-task-abc"), completer)

	err := service.ProcessIntegrationResult(context.Background(), CusdecIntegrationResultRequest{
		EdgeID: "edge-123", Integrated: true,
		Payload: cusdecResultPayload{CusdecRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 1}},
	})

	require.Error(t, err)
	assert.False(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusSubmitted, repo.declsByEdgeID["edge-123"].Status)
}

// Tasks that could not be read leave the declaration's status as it was and
// answer with an error, so SLC Edge's retry can still resume the parked step.
// Recording the status here would make that retry a duplicate.
func TestProcessEvent_TaskStoreFailureLeavesTheStatus(t *testing.T) {
	decl := &CusdecDeclaration{
		EdgeID: "edge-123", CusdecOffice: "COL", CusdecYear: "2026", CusdecSerial: "C", CusdecNumber: 9876,
		Status: CusdecStatusIntegrated, CaseID: testCaseID, WorkflowID: testWorkflowID,
	}
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": decl}}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, failingTasks{}, completer)

	err := service.ProcessEvent(context.Background(), CusdecEventRequest{
		Event:     "PAYMENT_CONFIRMED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef:  DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 9876},
			AmountPaid: 2035.00, Currency: "LKR",
		},
	})

	require.ErrorIs(t, err, errTaskStore)
	assert.False(t, repo.updateCalled, "the status must not be recorded while the step is unresumed")
	assert.Equal(t, CusdecStatusIntegrated, decl.Status)
	completer.AssertNotCalled(t, "CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// Tasks that could not be read are an error for the integration result too, not
// a declaration already processed.
func TestProcessCusdecIntegrationResult_TaskStoreFailure(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": submitted("edge-123")}}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, failingTasks{}, completer)

	err := service.ProcessIntegrationResult(context.Background(), CusdecIntegrationResultRequest{
		EdgeID: "edge-123", Integrated: true,
		Payload: cusdecResultPayload{CusdecRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 1}},
	})

	require.ErrorIs(t, err, errTaskStore)
	assert.NotErrorIs(t, err, ErrWorkflowNotFoundByEdgeID)
	assert.False(t, repo.updateCalled, "the result must not be recorded while the step is unresumed")
	assert.Equal(t, CusdecStatusSubmitted, repo.declsByEdgeID["edge-123"].Status)
	completer.AssertNotCalled(t, "CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The dispatch step records a declaration before SLC Edge can call back, so a
// result for an edgeId with no row is early: it is retried, and no declaration
// is created without the workflow it belongs to.
func TestProcessCusdecIntegrationResult_UnrecordedEdgeIDIsRetried(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{}}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, fakeTasks{}, completer)

	err := service.ProcessIntegrationResult(context.Background(), CusdecIntegrationResultRequest{
		EdgeID: "edge-unseen", Integrated: true,
		Payload: cusdecResultPayload{CusdecRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 1}},
	})

	require.ErrorIs(t, err, ErrDeclarationNotRecorded)
	assert.False(t, repo.updateCalled)
	completer.AssertNotCalled(t, "CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// A result for a declaration already past SUBMITTED is a repeat delivery: it is
// reported as a duplicate, for the handler to acknowledge, and neither changes
// the declaration nor resumes a task. A declaration the §6.5 events have moved
// on must not be put back to INTEGRATED.
func TestProcessCusdecIntegrationResult_RepeatIsDuplicate(t *testing.T) {
	for _, status := range []CusdecStatus{
		CusdecStatusIntegrated, CusdecStatusFailed, CusdecStatusPaid, CusdecStatusWarranted, CusdecStatusReleased,
	} {
		t.Run(string(status), func(t *testing.T) {
			decl := submitted("edge-123")
			decl.Status = status
			repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": decl}}
			completer := &mockTaskCompleter{}
			service := NewWebhookService(repo, parkedOn(reviewTemplateID, "task-abc", "step-task-abc"), completer)

			err := service.ProcessIntegrationResult(context.Background(), CusdecIntegrationResultRequest{
				EdgeID: "edge-123", Integrated: true,
				Payload: cusdecResultPayload{CusdecRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 1}},
			})

			require.ErrorIs(t, err, ErrDuplicateIntegrationResult)
			assert.False(t, repo.updateCalled)
			assert.Equal(t, status, decl.Status)
			completer.AssertNotCalled(t, "CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
