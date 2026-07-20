package asycuda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepository struct {
	notesByEdgID  map[string]*DispatchNote
	notesByCDNRef map[string]*DispatchNote
	updateCalled  bool
	updatedNote   *DispatchNote
	updateErr     error
}

func (m *mockRepository) GetByEdgID(ctx context.Context, edgID string) (*DispatchNote, error) {
	return m.notesByEdgID[edgID], nil
}

func (m *mockRepository) GetByCDNRef(ctx context.Context, ref DocumentReference) (*DispatchNote, error) {
	key := fmt.Sprintf("%s-%s-%s-%d", ref.Year, ref.Office, ref.Serial, ref.Number)
	return m.notesByCDNRef[key], nil
}

func (m *mockRepository) Update(ctx context.Context, note *DispatchNote) error {
	m.updateCalled = true
	m.updatedNote = note
	return m.updateErr
}

func TestProcessIntegrationResult_Success(t *testing.T) {
	ctx := context.Background()
	note := &DispatchNote{
		ID:     "note-123",
		EdgID:  "edg-123",
		Status: DispatchNoteStatusSubmitted,
	}

	repo := &mockRepository{
		notesByEdgID: map[string]*DispatchNote{
			"edg-123": note,
		},
	}
	service := NewCDNWebhookService(repo)

	req := CDNIntegrationResultRequest{
		EdgID:      "edg-123",
		Integrated: true,
		Event:      "INTEGRATION_RESULT",
		ProcessAt:  time.Now(),
		Payload: integrationResultPayload{
			CDNRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 4567,
			},
		},
	}

	err := service.ProcessIntegrationResult(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, DispatchNoteStatusIntegrated, repo.updatedNote.Status)
	assert.Equal(t, "2026", repo.updatedNote.CDNYear)
	assert.Equal(t, "COL", repo.updatedNote.CDNOffice)
	assert.Equal(t, "C", repo.updatedNote.CDNSerial)
	assert.Equal(t, 4567, repo.updatedNote.CDNNumber)
	assert.Empty(t, repo.updatedNote.Errors)
}

func TestProcessIntegrationResult_FailureWithErrorPersistence(t *testing.T) {
	ctx := context.Background()
	note := &DispatchNote{
		ID:     "note-123",
		EdgID:  "edg-123",
		Status: DispatchNoteStatusSubmitted,
	}

	repo := &mockRepository{
		notesByEdgID: map[string]*DispatchNote{
			"edg-123": note,
		},
	}
	service := NewCDNWebhookService(repo)

	rawErrors := json.RawMessage(`{"code":"ERR_VAL_01","message":"Invalid weight value"}`)
	req := CDNIntegrationResultRequest{
		EdgID:      "edg-123",
		Integrated: false,
		Event:      "INTEGRATION_RESULT",
		ProcessAt:  time.Now(),
		Errors:     rawErrors,
	}

	err := service.ProcessIntegrationResult(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, DispatchNoteStatusFailed, repo.updatedNote.Status)
	assert.JSONEq(t, string(rawErrors), string(repo.updatedNote.Errors))
}

func TestProcessIntegrationResult_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepository{
		notesByEdgID: map[string]*DispatchNote{},
	}
	service := NewCDNWebhookService(repo)

	req := CDNIntegrationResultRequest{
		EdgID:      "non-existent-edg",
		Integrated: true,
		Event:      "INTEGRATION_RESULT",
		ProcessAt:  time.Now(),
		Payload: integrationResultPayload{
			CDNRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 4567,
			},
		},
	}

	err := service.ProcessIntegrationResult(ctx, req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDispatchNoteNotFoundByEdgID))
}

func TestProcessAcknowledgment_Success(t *testing.T) {
	ctx := context.Background()
	note := &DispatchNote{
		ID:        "note-123",
		EdgID:     "edg-123",
		Status:    DispatchNoteStatusIntegrated,
		CDNYear:   "2026",
		CDNOffice: "COL",
		CDNSerial: "C",
		CDNNumber: 4567,
	}

	repo := &mockRepository{
		notesByCDNRef: map[string]*DispatchNote{
			"2026-COL-C-4567": note,
		},
	}
	service := NewCDNWebhookService(repo)

	req := CDNAcknowledgmentRequest{
		Event:     "ACKNOWLEDGMENT",
		ProcessAt: time.Now(),
		Payload: acknowledgmentPayload{
			CDNRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 4567,
			},
		},
	}

	err := service.ProcessAcknowledgment(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, DispatchNoteStatusAcknowledged, repo.updatedNote.Status)
}

func TestProcessAcknowledgment_InvalidState(t *testing.T) {
	ctx := context.Background()
	note := &DispatchNote{
		ID:        "note-123",
		EdgID:     "edg-123",
		Status:    DispatchNoteStatusSubmitted, // Invalid starting state for acknowledgment
		CDNYear:   "2026",
		CDNOffice: "COL",
		CDNSerial: "C",
		CDNNumber: 4567,
	}

	repo := &mockRepository{
		notesByCDNRef: map[string]*DispatchNote{
			"2026-COL-C-4567": note,
		},
	}
	service := NewCDNWebhookService(repo)

	req := CDNAcknowledgmentRequest{
		Event:     "ACKNOWLEDGMENT",
		ProcessAt: time.Now(),
		Payload: acknowledgmentPayload{
			CDNRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 4567,
			},
		},
	}

	err := service.ProcessAcknowledgment(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state transition")
	assert.False(t, repo.updateCalled)
}
