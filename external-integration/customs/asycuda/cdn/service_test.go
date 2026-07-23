package cdn

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepository struct {
	byEdgID  map[string]*DispatchNote
	byCDNRef map[string]*DispatchNote
	updated  *DispatchNote
}

func (m *mockRepository) GetByEdgID(ctx context.Context, edgID string) (*DispatchNote, error) {
	return m.byEdgID[edgID], nil
}

func (m *mockRepository) GetByCDNRef(ctx context.Context, ref DocumentReference) (*DispatchNote, error) {
	key := ref.Year + "-" + ref.Office + "-" + ref.Serial
	return m.byCDNRef[key], nil
}

func (m *mockRepository) Update(ctx context.Context, note *DispatchNote) error {
	m.updated = note
	return nil
}

func TestProcessIntegrationResult_Success(t *testing.T) {
	repo := &mockRepository{
		byEdgID: map[string]*DispatchNote{
			"edg-123": {ID: "1", EdgID: "edg-123", Status: DispatchNoteStatusSubmitted},
		},
	}
	svc := NewCDNWebhookService(repo)

	req := CDNIntegrationResultRequest{
		EdgID:      "edg-123",
		Integrated: true,
		Event:      "INTEGRATION_RESULT",
		Payload: integrationResultPayload{
			CDNRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 4567},
		},
	}

	err := svc.ProcessIntegrationResult(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, DispatchNoteStatusIntegrated, repo.updated.Status)
	assert.Equal(t, "COL", repo.updated.CDNOffice)
}

func TestProcessIntegrationResult_FailureWithErrorPersistence(t *testing.T) {
	repo := &mockRepository{
		byEdgID: map[string]*DispatchNote{
			"edg-123": {ID: "1", EdgID: "edg-123", Status: DispatchNoteStatusSubmitted},
		},
	}
	svc := NewCDNWebhookService(repo)

	rawErrors := json.RawMessage(`{"code":"ERR_VAL_01","message":"Invalid weight value"}`)
	req := CDNIntegrationResultRequest{
		EdgID:      "edg-123",
		Integrated: false,
		Event:      "INTEGRATION_RESULT",
		Errors:     rawErrors,
	}

	err := svc.ProcessIntegrationResult(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, DispatchNoteStatusFailed, repo.updated.Status)
	assert.JSONEq(t, string(rawErrors), string(repo.updated.Errors))
}

func TestProcessIntegrationResult_NotFound(t *testing.T) {
	repo := &mockRepository{byEdgID: map[string]*DispatchNote{}}
	svc := NewCDNWebhookService(repo)

	req := CDNIntegrationResultRequest{EdgID: "non-existent-edg"}
	err := svc.ProcessIntegrationResult(context.Background(), req)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDispatchNoteNotFoundByEdgID)
}

func TestProcessAcknowledgment_Success(t *testing.T) {
	repo := &mockRepository{
		byCDNRef: map[string]*DispatchNote{
			"2026-COL-C": {ID: "note-123", Status: DispatchNoteStatusIntegrated},
		},
	}
	svc := NewCDNWebhookService(repo)

	req := CDNAcknowledgmentRequest{
		Event: "ACKNOWLEDGMENT",
		Payload: acknowledgmentPayload{
			CDNRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 4567},
		},
	}

	err := svc.ProcessAcknowledgment(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, DispatchNoteStatusAcknowledged, repo.updated.Status)
}

func TestProcessAcknowledgment_InvalidState(t *testing.T) {
	repo := &mockRepository{
		byCDNRef: map[string]*DispatchNote{
			"2026-COL-C": {ID: "note-123", Status: DispatchNoteStatusSubmitted},
		},
	}
	svc := NewCDNWebhookService(repo)

	req := CDNAcknowledgmentRequest{
		Event: "ACKNOWLEDGMENT",
		Payload: acknowledgmentPayload{
			CDNRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 4567},
		},
	}

	err := svc.ProcessAcknowledgment(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot acknowledge dispatch note in status SUBMITTED")
}
