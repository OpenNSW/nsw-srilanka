package asycuda

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
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

type mockAuditor struct {
	events []*argus.AuditLogRequest
}

func (m *mockAuditor) IsEnabled() bool { return true }

func (m *mockAuditor) LogEvent(_ context.Context, event *argus.AuditLogRequest) bool {
	m.events = append(m.events, event)
	return true
}

func (m *mockAuditor) SignEvent(context.Context, *argus.AuditLogRequest) error { return nil }

func (m *mockAuditor) SignMessageBytes(context.Context, []byte) (string, error) { return "", nil }

func (m *mockAuditor) LogSignedEvent(context.Context, *argus.AuditLogRequest) {}

func (m *mockAuditor) VerifyIntegrity(*argus.AuditLogRequest, crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditor) Close(context.Context) error { return nil }

func newEnabledSLCERecorder() (*nswaudit.Recorder, *mockAuditor) {
	auditor := &mockAuditor{}
	return nswaudit.NewRecorder(auditor), auditor
}

func assertConsignmentAuditEvent(t *testing.T, event *argus.AuditLogRequest, wantStatus int, wantEventType string, wantFailure bool) {
	t.Helper()
	require.NotNil(t, event)
	assert.Equal(t, string(nswaudit.EventConsignment), event.EventType)
	assert.Equal(t, string(nswaudit.ActionUpdate), event.Action)
	assert.Equal(t, string(nswaudit.TargetConsignment), event.TargetType)
	assert.Equal(t, wantEventType, event.Metadata["eventType"])
	assert.Equal(t, wantStatus, event.Metadata["status"])
	if wantFailure {
		assert.Equal(t, argus.StatusFailure, event.Status)
	} else {
		assert.Equal(t, argus.StatusSuccess, event.Status)
	}
}

func TestSLCEHandler_Audit_CusdecIntegrationSuccess(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	recorder, auditor := newEnabledSLCERecorder()
	handler := NewHandler(cusdecSvc, cdnSvc, recorder)

	payload := `{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-07-23T11:00:00Z",
		"payload": {
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": true,
			"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 },
			"taxes": [],
			"errors": {}
		}
	}`

	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()
	handler.HandleWebhook(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, auditor.events, 1)
	assertConsignmentAuditEvent(t, auditor.events[0], http.StatusOK, "CUSDEC_INTEGRATED", false)
}

func TestSLCEHandler_Audit_CusdecIntegrationFailure(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	recorder, auditor := newEnabledSLCERecorder()
	handler := NewHandler(cusdecSvc, cdnSvc, recorder)

	payload := `{"eventType": "UNKNOWN_EVENT_TYPE", "processedAt": "2026-07-23T10:00:00Z"}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()
	handler.HandleWebhook(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Len(t, auditor.events, 1)
	assertConsignmentAuditEvent(t, auditor.events[0], http.StatusBadRequest, "UNKNOWN_EVENT_TYPE", true)
}

func TestSLCEHandler_Audit_CusdecEventDispatch(t *testing.T) {
	tests := []struct {
		name            string
		rawEventType    string
		normalizedEvent string
		payload         string
		setupMock       func(cusdecSvc *mockCusdecService)
		wantStatus      int
		wantFailure     bool
	}{
		{
			name:            "PAYMENT_CONFIRMED success",
			rawEventType:    "PAYMENT_CONFIRMED",
			normalizedEvent: "PAYMENT_CONFIRMED",
			payload: `{
				"eventType": "PAYMENT_CONFIRMED",
				"processedAt": "2026-07-23T11:05:00Z",
				"payload": {
					"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 }
				}
			}`,
			setupMock: func(cusdecSvc *mockCusdecService) {
				cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
					return r.Event == "PAYMENT_CONFIRMED"
				})).Return(nil)
			},
			wantStatus:  http.StatusOK,
			wantFailure: false,
		},
		{
			name:            "normalized mixed-case event type",
			rawEventType:    "  payment_confirmed  ",
			normalizedEvent: "PAYMENT_CONFIRMED",
			payload: `{
				"eventType": "  payment_confirmed  ",
				"processedAt": "2026-07-23T11:05:00Z",
				"payload": {
					"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 }
				}
			}`,
			setupMock: func(cusdecSvc *mockCusdecService) {
				cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
					return r.Event == "PAYMENT_CONFIRMED"
				})).Return(nil)
			},
			wantStatus:  http.StatusOK,
			wantFailure: false,
		},
		{
			name:            "EXPORT_RELEASED service failure",
			rawEventType:    "EXPORT_RELEASED",
			normalizedEvent: "EXPORT_RELEASED",
			payload: `{
				"eventType": "EXPORT_RELEASED",
				"processedAt": "2026-07-23T11:15:00Z",
				"payload": {
					"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 }
				}
			}`,
			setupMock: func(cusdecSvc *mockCusdecService) {
				cusdecSvc.On("ProcessEvent", mock.Anything, mock.Anything).Return(cusdec.ErrCusdecNotFoundByRef)
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cusdecSvc := new(mockCusdecService)
			cdnSvc := new(mockCDNService)
			recorder, auditor := newEnabledSLCERecorder()
			handler := NewHandler(cusdecSvc, cdnSvc, recorder)

			tt.setupMock(cusdecSvc)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tt.payload))
			w := httptest.NewRecorder()
			handler.HandleWebhook(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Len(t, auditor.events, 1)
			assertConsignmentAuditEvent(t, auditor.events[0], tt.wantStatus, tt.normalizedEvent, tt.wantFailure)
			cusdecSvc.AssertExpectations(t)
		})
	}
}

// Tests CusDec integration result success (v1.2 §6.2).
func TestSLCEHandler_CusdecIntegrationResultSuccess(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc, nil)

	payload := `{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-07-23T11:00:00Z",
		"payload": {
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": true,
			"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 },
			"taxes": [
				{ "code": "tax1", "rate": 1, "amount": 222 },
				{ "code": "tax2", "rate": 1, "amount": 1022 }
			],
			"errors": {}
		}
	}`

	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecIntegrationResultRequest) bool {
		return r.EdgeID == "5516e4c8-a93d-429d-8a18-6a484d331176" &&
			r.Integrated &&
			r.Event == "CUSDEC_INTEGRATED" &&
			r.Payload.CusdecRef.Office == "CMB" &&
			r.Payload.CusdecRef.Number == 1001 &&
			len(r.Payload.Taxes) == 2
	})).Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	cusdecSvc.AssertExpectations(t)
}

// Tests CusDec status event notifications success paths (v1.2 §6.5).
func TestSLCEHandler_CusdecEventsSuccess(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		payload   string
		setupMock func(cusdecSvc *mockCusdecService)
	}{
		{
			name:      "2. PAYMENT_CONFIRMED (§6.5.1)",
			eventType: "PAYMENT_CONFIRMED",
			payload: `{
				"eventType": "PAYMENT_CONFIRMED",
				"processedAt": "2026-07-23T11:05:00Z",
				"payload": {
					"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 },
					"amountPaid": 2035.00,
					"currency": "LKR",
					"bankReference": "BNK-2026-0098765"
				}
			}`,
			setupMock: func(cusdecSvc *mockCusdecService) {
				cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
					return r.Event == "PAYMENT_CONFIRMED" &&
						r.Payload.CusdecRef.Office == "CMB" &&
						r.Payload.CusdecRef.Number == 1001 &&
						r.Payload.AmountPaid == 2035.00 &&
						r.Payload.Currency == "LKR"
				})).Return(nil)
			},
		},
		{
			name:      "3. WARRANTING_COMPLETED (§6.5.2)",
			eventType: "WARRANTING_COMPLETED",
			payload: `{
				"eventType": "WARRANTING_COMPLETED",
				"processedAt": "2026-07-23T11:10:00Z",
				"payload": {
					"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 },
					"releaseOrderNo": "RO/2026/004567",
					"examinationRequired": false
				}
			}`,
			setupMock: func(cusdecSvc *mockCusdecService) {
				cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
					return r.Event == "WARRANTING_COMPLETED" &&
						r.Payload.CusdecRef.Office == "CMB" &&
						r.Payload.CusdecRef.Number == 1001 &&
						r.Payload.ReleaseOrderNo == "RO/2026/004567"
				})).Return(nil)
			},
		},
		{
			name:      "4. EXPORT_RELEASED (§6.5.3)",
			eventType: "EXPORT_RELEASED",
			payload: `{
				"eventType": "EXPORT_RELEASED",
				"processedAt": "2026-07-23T11:15:00Z",
				"payload": {
					"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 },
					"vesselName": "EVER GIVEN",
					"voyageNo": "023W",
					"portOfLoading": "LKCMB"
				}
			}`,
			setupMock: func(cusdecSvc *mockCusdecService) {
				cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
					return r.Event == "EXPORT_RELEASED" &&
						r.Payload.CusdecRef.Office == "CMB" &&
						r.Payload.CusdecRef.Number == 1001 &&
						r.Payload.VesselName == "EVER GIVEN" &&
						r.Payload.PortOfLoading == "LKCMB"
				})).Return(nil)
			},
		},
		{
			name:      "Case insensitive event type handling",
			eventType: "  payment_confirmed  ",
			payload: `{
				"eventType": "  payment_confirmed  ",
				"processedAt": "2026-07-23T11:05:00Z",
				"payload": { "cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 } }
			}`,
			setupMock: func(cusdecSvc *mockCusdecService) {
				cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
					return r.Event == "PAYMENT_CONFIRMED"
				})).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cusdecSvc := new(mockCusdecService)
			cdnSvc := new(mockCDNService)
			handler := NewHandler(cusdecSvc, cdnSvc, nil)

			tt.setupMock(cusdecSvc)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleWebhook(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			cusdecSvc.AssertExpectations(t)
		})
	}
}

// Tests all Cargo Dispatch Note webhook event success paths (v1.2 §7).
func TestSLCEHandler_CDNSuccessEvents(t *testing.T) {
	t.Run("5. CDN_INTEGRATED (§7.2)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc, nil)

		payload := `{
			"eventType": "CDN_INTEGRATED",
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": true,
			"processedAt": "2026-07-23T11:20:00Z",
			"payload": {
				"cdnRef": { "year": "2026", "office": "CMB", "serial": "D", "number": 2002 }
			},
			"errors": {}
		}`

		cdnSvc.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cdn.CDNIntegrationResultRequest) bool {
			return r.EdgeID == "5516e4c8-a93d-429d-8a18-6a484d331176" &&
				r.Integrated &&
				r.Event == "CDN_INTEGRATED" &&
				r.Payload.CDNRef.Office == "CMB" &&
				r.Payload.CDNRef.Number == 2002
		})).Return(nil)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		cdnSvc.AssertExpectations(t)
	})

	t.Run("6. CDN_ACKNOWLEDGED (§7.3)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc, nil)

		payload := `{
			"eventType": "CDN_ACKNOWLEDGED",
			"processedAt": "2026-07-23T11:25:00Z",
			"payload": {
				"cdnRef": { "year": "2026", "office": "CMB", "serial": "D", "number": 2002 }
			}
		}`

		cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.MatchedBy(func(r cdn.CDNAcknowledgmentRequest) bool {
			return r.Event == "CDN_ACKNOWLEDGED" &&
				r.Payload.CDNRef.Office == "CMB" &&
				r.Payload.CDNRef.Number == 2002
		})).Return(nil)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		cdnSvc.AssertExpectations(t)
	})
}

// Tests request validation failure paths for all event types.
func TestSLCEHandler_ValidationFailures(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "CusDec result missing edgeId",
			payload: `{
				"eventType": "CUSDEC_INTEGRATED",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {"edgeId": "", "integrated": true}
			}`,
		},
		{
			name: "CusDec result integrated true but missing cusDecRef",
			payload: `{
				"eventType": "CUSDEC_INTEGRATED",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {"edgeId": "edge-123", "integrated": true}
			}`,
		},
		{
			name: "CusDec event missing cusDecRef",
			payload: `{
				"eventType": "PAYMENT_CONFIRMED",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {}
			}`,
		},
		{
			name: "CDN result missing edgeId",
			payload: `{
				"edgeId": "",
				"integrated": true,
				"eventType": "CDN_INTEGRATED",
				"processedAt": "2026-07-23T10:00:00Z"
			}`,
		},
		{
			name: "CDN ack missing cdnRef",
			payload: `{
				"eventType": "CDN_ACKNOWLEDGED",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": {}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cusdecSvc := new(mockCusdecService)
			cdnSvc := new(mockCDNService)
			handler := NewHandler(cusdecSvc, cdnSvc, nil)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleWebhook(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// Tests error propagation and HTTP status code mapping.
func TestSLCEHandler_ErrorResponses(t *testing.T) {
	t.Run("Unknown event type", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc, nil)

		payload := `{"eventType": "UNKNOWN_EVENT_TYPE", "processedAt": "2026-07-23T10:00:00Z"}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "unknown or unsupported event type")
	})

	t.Run("Invalid JSON body", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc, nil)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(`{invalid-json`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Workflow not found by edgeId (404 Not Found)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc, nil)

		payload := `{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-07-23T10:00:00Z",
			"payload": {
				"edgeId": "edge-missing",
				"integrated": true,
				"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
			}
		}`

		cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrWorkflowNotFoundByEdgeID)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Cusdec not found by reference (503 Service Unavailable)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc, nil)

		payload := `{
			"eventType": "PAYMENT_CONFIRMED",
			"processedAt": "2026-07-23T10:00:00Z",
			"payload": {"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}
		}`

		cusdecSvc.On("ProcessEvent", mock.Anything, mock.Anything).Return(cusdec.ErrCusdecNotFoundByRef)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("Internal DB failure (500 Internal Server Error)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc, nil)

		payload := `{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-07-23T10:00:00Z",
			"payload": {
				"edgeId": "edge-err",
				"integrated": true,
				"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
			}
		}`

		cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(errors.New("db connection failure"))

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "An error occurred while processing your request")
	})
}
