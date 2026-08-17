package autogen

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/taskflow/store"
)

type mockSeqService struct {
	expectedRef string
	err         error
}

func (m *mockSeqService) GenerateID(ctx context.Context, issuer, idType string, params map[string]string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.expectedRef, nil
}

func (m *mockSeqService) GetOrGenerateID(ctx context.Context, rootWorkflowID, parentWorkflowID, issuer, idType, targetKey string, params map[string]string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.expectedRef, nil
}

func (m *mockSeqService) GenerateNext(ctx context.Context, agencyCode string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.expectedRef, nil
}

func (m *mockSeqService) GetOrGenerateNext(ctx context.Context, rootWorkflowID, parentWorkflowID, agencyCode, targetKey string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.expectedRef, nil
}

func TestAutogenExtension_Execute(t *testing.T) {
	mockSvc := &mockSeqService{expectedRef: "034/00001"}
	ext := NewExtension(mockSvc)

	record := &store.TaskRecord{
		TaskID:   "task-123",
		TaskType: "fcau_application_review_v1",
		Data:     make(map[string]any),
	}

	props := json.RawMessage(`{"agency_code":"FCAU"}`)
	err := ext.Execute(context.Background(), record, nil, props)
	require.NoError(t, err)

	assert.Equal(t, "034/00001", record.Data["reference_number"])
}

func TestAutogenExtension_CustomTargetKey(t *testing.T) {
	mockSvc := &mockSeqService{expectedRef: "034/00002"}
	ext := NewExtension(mockSvc)

	record := &store.TaskRecord{
		TaskID:   "task-124",
		TaskType: "fcau_application_review_v1",
		Data:     make(map[string]any),
	}

	props := json.RawMessage(`{"agency_code":"FCAU","target_key":"application_id"}`)
	err := ext.Execute(context.Background(), record, nil, props)
	require.NoError(t, err)

	assert.Equal(t, "034/00002", record.Data["application_id"])
}

func TestAutogenExtension_InfersAgencyCode(t *testing.T) {
	mockSvc := &mockSeqService{expectedRef: "045/00001"}
	ext := NewExtension(mockSvc)

	record := &store.TaskRecord{
		TaskID:   "task-456",
		TaskType: "npqs_inspection_review_v1",
		Data:     make(map[string]any),
	}

	err := ext.Execute(context.Background(), record, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "045/00001", record.Data["reference_number"])
}

func TestAutogenExtension_DoesNotOverwriteExisting(t *testing.T) {
	mockSvc := &mockSeqService{expectedRef: "034/00099"}
	ext := NewExtension(mockSvc)

	record := &store.TaskRecord{
		TaskID:   "task-789",
		TaskType: "fcau_review_v1",
		Data: map[string]any{
			"reference_number": "034/00005",
		},
	}

	err := ext.Execute(context.Background(), record, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "034/00005", record.Data["reference_number"])
}
