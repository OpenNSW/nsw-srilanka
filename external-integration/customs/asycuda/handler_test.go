package asycuda

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

// mockCusdecService is a mock implementation of cusdec.WebhookService.
type mockCusdecService struct {
	mock.Mock
}

func (m *mockCusdecService) ProcessIntegrationResult(ctx context.Context, req cusdec.CusdecIntegrationResultRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *mockCusdecService) ProcessEvent(ctx context.Context, req cusdec.CusdecEventRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

// mockCDNService is a mock implementation of cdn.CDNWebhookService.
type mockCDNService struct {
	mock.Mock
}

func (m *mockCDNService) ProcessIntegrationResult(ctx context.Context, req cdn.CDNIntegrationResultRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *mockCDNService) ProcessAcknowledgment(ctx context.Context, req cdn.CDNAcknowledgmentRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func TestSLCEHandler_CusdecIntegrationResult(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	payload := `{
		"edgeId": "edg-101",
		"integrated": true,
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-23T10:00:00Z",
		"payload": {
			"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`

	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecIntegrationResultRequest) bool {
		return r.EdgeID == "edg-101" && r.Integrated && r.Payload.CusdecRef.Office == "CBEX1"
	})).Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	cusdecSvc.AssertExpectations(t)
}

func TestSLCEHandler_CusdecEvents(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		expectedEvent string
	}{
		{
			name: "PAYMENT event",
			payload: `{
				"event": "PAYMENT",
				"processAt": "2026-07-23T10:00:00Z",
				"payload": {"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
			}`,
			expectedEvent: "PAYMENT",
		},
		{
			name: "PAYMENT_NOTIFICATION event",
			payload: `{
				"eventType": "PAYMENT_NOTIFICATION",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
			}`,
			expectedEvent: "PAYMENT",
		},
		{
			name: "WARRANTING event",
			payload: `{
				"event": "WARRANTING",
				"processAt": "2026-07-23T10:00:00Z",
				"payload": {"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
			}`,
			expectedEvent: "WARRANTING",
		},
		{
			name: "WARRANTING_NOTIFICATION event",
			payload: `{
				"eventType": "WARRANTING_NOTIFICATION",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
			}`,
			expectedEvent: "WARRANTING",
		},
		{
			name: "RELEASE event",
			payload: `{
				"event": "RELEASE",
				"processAt": "2026-07-23T10:00:00Z",
				"payload": {"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
			}`,
			expectedEvent: "RELEASE",
		},
		{
			name: "RELEASE_NOTIFICATION event",
			payload: `{
				"eventType": "RELEASE_NOTIFICATION",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
			}`,
			expectedEvent: "RELEASE",
		},
		{
			name: "Case insensitive and trimmed event",
			payload: `{
				"eventType": "  payment_notification  ",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
			}`,
			expectedEvent: "PAYMENT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cusdecSvc := new(mockCusdecService)
			cdnSvc := new(mockCDNService)
			handler := NewHandler(cusdecSvc, cdnSvc)

			cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
				return r.Event == tt.expectedEvent && r.Payload.CusdecRef.Office == "CBEX1"
			})).Return(nil)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleWebhook(w, req)

			assert.Equal(t, http.StatusAccepted, w.Code)
			cusdecSvc.AssertExpectations(t)
		})
	}
}

func TestSLCEHandler_CDNIntegrationResult(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	payload := `{
		"edgId": "cdn-edg-99",
		"integrated": true,
		"event": "CDN_INTEGRATION_RESULT",
		"processAt": "2026-07-23T10:00:00Z",
		"payload": {
			"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 1002}
		}
	}`

	cdnSvc.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cdn.CDNIntegrationResultRequest) bool {
		return r.EdgID == "cdn-edg-99" && r.Payload.CDNRef.Office == "CBEX1"
	})).Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	cdnSvc.AssertExpectations(t)
}

func TestSLCEHandler_CDNAcknowledgment(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "ACKNOWLEDGMENT event",
			payload: `{
				"event": "ACKNOWLEDGMENT",
				"processAt": "2026-07-23T10:00:00Z",
				"payload": {"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 1002}}
			}`,
		},
		{
			name: "CDN_ACKNOWLEDGMENT event",
			payload: `{
				"eventType": "CDN_ACKNOWLEDGMENT",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 1002}}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cusdecSvc := new(mockCusdecService)
			cdnSvc := new(mockCDNService)
			handler := NewHandler(cusdecSvc, cdnSvc)

			cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.MatchedBy(func(r cdn.CDNAcknowledgmentRequest) bool {
				return r.Payload.CDNRef.Office == "CBEX1"
			})).Return(nil)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleWebhook(w, req)

			assert.Equal(t, http.StatusAccepted, w.Code)
			cdnSvc.AssertExpectations(t)
		})
	}
}

func TestSLCEHandler_ValidationFailures(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "CusDec result missing edgeId",
			payload: `{
				"edgeId": "",
				"integrated": true,
				"event": "INTEGRATION_RESULT",
				"processAt": "2026-07-23T10:00:00Z"
			}`,
		},
		{
			name: "CusDec result integrated true but missing cusDecRef",
			payload: `{
				"edgeId": "edg-123",
				"integrated": true,
				"event": "INTEGRATION_RESULT",
				"processAt": "2026-07-23T10:00:00Z",
				"payload": {}
			}`,
		},
		{
			name: "CusDec event missing cusDecRef",
			payload: `{
				"event": "PAYMENT",
				"processAt": "2026-07-23T10:00:00Z",
				"payload": {}
			}`,
		},
		{
			name: "CDN result missing edgId",
			payload: `{
				"edgId": "",
				"integrated": true,
				"event": "CDN_INTEGRATION_RESULT",
				"processAt": "2026-07-23T10:00:00Z"
			}`,
		},
		{
			name: "CDN ack missing cdnRef",
			payload: `{
				"event": "ACKNOWLEDGMENT",
				"processAt": "2026-07-23T10:00:00Z",
				"payload": {}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cusdecSvc := new(mockCusdecService)
			cdnSvc := new(mockCDNService)
			handler := NewHandler(cusdecSvc, cdnSvc)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleWebhook(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestSLCEHandler_InternalServerErrors(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	payload := `{
		"edgeId": "edg-err",
		"integrated": true,
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-23T10:00:00Z",
		"payload": {
			"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`

	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(errors.New("db connection failure"))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "internal server error")
}

func TestSLCEHandler_UnknownEvent(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	payload := `{"event": "UNKNOWN_EVENT_TYPE", "processAt": "2026-07-23T10:00:00Z"}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unknown or unsupported event type")
}

func TestSLCEHandler_InvalidJSON(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(`{invalid-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSLCEHandler_TransientServiceUnavailable(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	payload := `{
		"event": "PAYMENT",
		"processAt": "2026-07-23T10:00:00Z",
		"payload": {"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
	}`

	cusdecSvc.On("ProcessEvent", mock.Anything, mock.Anything).Return(cusdec.ErrCusdecNotFoundByRef)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSLCEHandler_WorkflowNotFound(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	payload := `{
		"edgeId": "edg-missing",
		"integrated": true,
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-23T10:00:00Z",
		"payload": {
			"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`

	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrWorkflowNotFoundByEdgeID)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
