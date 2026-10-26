package cdn

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

func amendment(t *testing.T, operationTime string) CDNAmendedRequest {
	t.Helper()
	var req CDNAmendedRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"eventType": "CDN_AMENDED",
		"processedAt": "2026-04-27T14:30:10Z",
		"payload": {
			"cdnRef": {"office": "CBEX1", "year": "2026", "serial": "C", "number": 28237},
			"cdn": {
				"properties": {"submitter": "", "nswId": ""},
				"containerNumber": "MSCU1234567",
				"cusDecRefs": [{"office": "CBEX1", "year": "2026", "serial": "E", "number": 1047}]
			},
			"operationTime": "`+operationTime+`"
		}
	}`), &req))
	require.NoError(t, req.Validate())
	return req
}

// registeredNotes holds one integrated note under the reference the amendment
// names (the mock keys on year-office-serial).
func registeredNotes() map[string]*DispatchNote {
	return map[string]*DispatchNote{
		"2026-CBEX1-C": {
			ID: "note-1", EdgeID: "edge-123", Status: DispatchNoteStatusAcknowledged, CaseID: "case-1", WorkflowID: "wf-1",
			CDNOffice: "CBEX1", CDNYear: "2026", CDNSerial: "C", CDNNumber: 28237,
		},
	}
}

// An amendment opens the amendment flow on the note's case, seeded with the
// note as amended.
func TestProcessAmendment_OpensTheAmendmentFlow(t *testing.T) {
	repo := &mockRepository{byCDNRef: registeredNotes()}
	flows := &recordingFlows{}
	svc := NewCDNWebhookService(repo, nil, nil, flows)

	require.NoError(t, svc.ProcessAmendment(context.Background(), amendment(t, "2026-10-14T05:07:38Z")))

	require.Len(t, flows.started, 1)
	got := flows.started[0]
	assert.Equal(t, "case-1", got.caseID)
	assert.Equal(t, AmendedFlowTemplateID, got.templateID)
	assert.Equal(t, "case-1", got.vars["consignmentId"])
	assert.Equal(t, "CBEX1/2026/C/28237", got.vars["cdn_number"])
	assert.Equal(t, "2026-10-14T05:07:38Z", got.vars["amended_at"])
	assert.Equal(t, "MSCU1234567", got.vars["cdn"].(map[string]any)["containerNumber"])
	assert.Nil(t, repo.updated, "an amendment does not change the note's own lifecycle")
}

// SLC Edge retries a notification (§2): every delivery of one amendment names
// the same flow, so it is opened once. A later amendment opens a flow of its own.
func TestProcessAmendment_OneFlowPerAmendment(t *testing.T) {
	flows := &recordingFlows{}
	svc := NewCDNWebhookService(&mockRepository{byCDNRef: registeredNotes()}, nil, nil, flows)
	ctx := context.Background()

	require.NoError(t, svc.ProcessAmendment(ctx, amendment(t, "2026-10-14T05:07:38Z")))
	require.NoError(t, svc.ProcessAmendment(ctx, amendment(t, "2026-10-14T05:07:38Z")))
	require.NoError(t, svc.ProcessAmendment(ctx, amendment(t, "2026-10-15T09:00:00Z")))

	require.Len(t, flows.started, 3)
	assert.Equal(t, flows.started[0].workflowID, flows.started[1].workflowID, "a redelivery names the same flow")
	assert.NotEqual(t, flows.started[0].workflowID, flows.started[2].workflowID, "a later amendment is a new flow")
}

func TestProcessAmendment_Errors(t *testing.T) {
	unlinked := registeredNotes()
	unlinked["2026-CBEX1-C"].CaseID = ""
	for name, tc := range map[string]struct {
		notes map[string]*DispatchNote
		want  error
	}{
		"reference not registered": {notes: map[string]*DispatchNote{}, want: ErrDispatchNoteNotFoundByCDNRef},
		"no case recorded":         {notes: unlinked, want: ErrCaseNotRecorded},
	} {
		t.Run(name, func(t *testing.T) {
			flows := &recordingFlows{}
			svc := NewCDNWebhookService(&mockRepository{byCDNRef: tc.notes}, nil, nil, flows)

			err := svc.ProcessAmendment(context.Background(), amendment(t, "2026-10-14T05:07:38Z"))

			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, flows.started)
		})
	}
}

func TestCDNAmendedRequest_Validate(t *testing.T) {
	for name, mutate := range map[string]func(*CDNAmendedRequest){
		"wrong event":       func(r *CDNAmendedRequest) { r.EventType = "CDN_CANCELLED" },
		"partial reference": func(r *CDNAmendedRequest) { r.Payload.CDNRef.Serial = "" },
		"note not object":   func(r *CDNAmendedRequest) { r.Payload.CDN = json.RawMessage(`[]`) },
		"no note":           func(r *CDNAmendedRequest) { r.Payload.CDN = nil },
		"no operation time": func(r *CDNAmendedRequest) { r.Payload.OperationTime = " " },
	} {
		t.Run(name, func(t *testing.T) {
			req := amendment(t, "2026-10-14T05:07:38Z")
			mutate(&req)
			assert.Error(t, req.Validate())
		})
	}
}
