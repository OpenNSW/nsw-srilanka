package cusdec

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type startedFlow struct {
	caseID, templateID, workflowID string
	vars                           map[string]any
}

// recordingFlows records the flows started on cases.
type recordingFlows struct{ started []startedFlow }

func (r *recordingFlows) StartFlow(_ context.Context, caseID, templateID, workflowID string, vars map[string]any) error {
	r.started = append(r.started, startedFlow{caseID, templateID, workflowID, vars})
	return nil
}

var amendedRef = DocumentReference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 1047}

func amendment(t *testing.T, operationTime string) CusdecAmendedRequest {
	t.Helper()
	var req CusdecAmendedRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"eventType": "CUSDEC_AMENDED",
		"processedAt": "2026-04-26T11:15:22Z",
		"payload": {
			"cusDecRef": {"office": "CBEX1", "year": "2026", "serial": "E", "number": 1047},
			"cusDec": {"properties": {"submitter": "", "nswId": ""}, "generalSegment": {"countryOfDestination": "DE"}},
			"duties": {
				"globalDuties": [{"typeCode": "EPF", "taxBaseAmount": 550, "taxRateNumeric": 2.0, "taxAssessedAmount": 1100, "paymentMethodCode": "1"}],
				"itemDutiesList": [{"itemSequenceNumeric": 1, "dutyTaxFees": [{"typeCode": "CED", "taxBaseAmount": 125, "taxRateNumeric": 0.0, "taxAssessedAmount": 0, "paymentMethodCode": "1"}]}]
			},
			"operationTime": "`+operationTime+`"
		}
	}`), &req))
	require.NoError(t, req.Validate())
	return req
}

func registeredDeclaration() *CusdecDeclaration {
	return &CusdecDeclaration{
		EdgeID: "edge-123", Status: CusdecStatusPaid, CaseID: testCaseID, WorkflowID: testWorkflowID,
		CusdecOffice: amendedRef.Office, CusdecYear: amendedRef.Year, CusdecSerial: amendedRef.Serial, CusdecNumber: amendedRef.Number,
	}
}

// An amendment opens the amendment flow on the declaration's case, seeded with
// the declaration as amended and its reassessed duties.
func TestProcessAmendment_OpensTheAmendmentFlow(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": registeredDeclaration()}}
	flows := &recordingFlows{}
	service := NewWebhookService(repo, fakeTasks{}, &mockTaskCompleter{}, flows)

	require.NoError(t, service.ProcessAmendment(context.Background(), amendment(t, "2026-10-14T05:07:38Z")))

	require.Len(t, flows.started, 1)
	got := flows.started[0]
	assert.Equal(t, testCaseID, got.caseID)
	assert.Equal(t, AmendedFlowTemplateID, got.templateID)
	assert.Equal(t, testCaseID, got.vars["consignmentId"])
	assert.Equal(t, "CBEX1/2026/E/1047", got.vars["cusdec_number"])
	assert.Equal(t, "2026-10-14T05:07:38Z", got.vars["amended_at"])
	assert.Equal(t, "DE", got.vars["cusdec"].(map[string]any)["generalSegment"].(map[string]any)["countryOfDestination"])
	duties := got.vars["duties"].(map[string]any)
	assert.Len(t, duties["globalDuties"], 1)
	assert.Len(t, duties["itemDutiesList"], 1)
	assert.False(t, repo.updateCalled, "an amendment does not change the declaration's own lifecycle")
}

// SLC Edge retries a notification (§2): every delivery of one amendment names
// the same flow, so it is opened once. A later amendment opens a flow of its own.
func TestProcessAmendment_OneFlowPerAmendment(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": registeredDeclaration()}}
	flows := &recordingFlows{}
	service := NewWebhookService(repo, fakeTasks{}, &mockTaskCompleter{}, flows)
	ctx := context.Background()

	require.NoError(t, service.ProcessAmendment(ctx, amendment(t, "2026-10-14T05:07:38Z")))
	require.NoError(t, service.ProcessAmendment(ctx, amendment(t, "2026-10-14T05:07:38Z")))
	require.NoError(t, service.ProcessAmendment(ctx, amendment(t, "2026-10-15T09:00:00Z")))

	require.Len(t, flows.started, 3)
	assert.Equal(t, flows.started[0].workflowID, flows.started[1].workflowID, "a redelivery names the same flow")
	assert.NotEqual(t, flows.started[0].workflowID, flows.started[2].workflowID, "a later amendment is a new flow")
}

func TestProcessAmendment_Errors(t *testing.T) {
	unlinked := registeredDeclaration()
	unlinked.CaseID = ""
	for name, tc := range map[string]struct {
		decls map[string]*CusdecDeclaration
		want  error
	}{
		"reference not registered": {decls: map[string]*CusdecDeclaration{}, want: ErrCusdecNotFoundByRef},
		"no case recorded":         {decls: map[string]*CusdecDeclaration{"edge-123": unlinked}, want: ErrCaseNotRecorded},
	} {
		t.Run(name, func(t *testing.T) {
			flows := &recordingFlows{}
			service := NewWebhookService(&mockCusdecRepository{declsByEdgeID: tc.decls}, fakeTasks{}, &mockTaskCompleter{}, flows)

			err := service.ProcessAmendment(context.Background(), amendment(t, "2026-10-14T05:07:38Z"))

			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, flows.started)
		})
	}
}
