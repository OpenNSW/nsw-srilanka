package asycuda

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (m *mockCusdecService) ProcessAmendment(ctx context.Context, req cusdec.CusdecAmendedRequest) error {
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

func (m *mockCDNService) ProcessAmendment(ctx context.Context, req cdn.CDNAmendedRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *mockCDNService) ProcessCancellation(ctx context.Context, req cdn.CDNCancelledRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

// Tests CusDec integration result success (v1.2 §6.2).
func TestSLCEHandler_CusdecIntegrationResultSuccess(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cdnSvc := new(mockCDNService)
	handler := NewHandler(cusdecSvc, cdnSvc)

	payload := `{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-07-23T11:00:00Z",
		"payload": {
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": true,
			"totalAssessedAmount": 1244,
			"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 },
			"amountPayable": 1244,
			"duties": {
				"globalDuties": [
					{ "typeCode": "EPF", "taxBaseAmount": 550, "taxRateNumeric": 2.0, "taxAssessedAmount": 1100, "paymentMethodCode": "1" },
					{ "typeCode": "COM", "taxBaseAmount": 1, "taxRateNumeric": 144.0, "taxAssessedAmount": 144, "paymentMethodCode": "1" }
				]
			},
			"errors": {}
		}
	}`

	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecIntegrationResultRequest) bool {
		return r.EdgeID == "5516e4c8-a93d-429d-8a18-6a484d331176" &&
			r.Integrated &&
			r.Event == "CUSDEC_INTEGRATED" &&
			r.Payload.CusdecRef.Office == "CMB" &&
			r.Payload.CusdecRef.Number == 1001 &&
			len(r.Payload.Duties.GlobalDuties) == 2
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
			handler := NewHandler(cusdecSvc, cdnSvc)

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
		handler := NewHandler(cusdecSvc, cdnSvc)

		payload := `{
			"eventType": "CDN_INTEGRATED",
			"processedAt": "2026-07-23T11:20:00Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": true,
				"cdnRef": { "year": "2026", "office": "CMB", "serial": "D", "number": 2002 },
				"errors": {}
			}
		}`

		cdnSvc.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cdn.CDNIntegrationResultRequest) bool {
			return r.Payload.EdgeID == "5516e4c8-a93d-429d-8a18-6a484d331176" &&
				r.Payload.Integrated &&
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
		handler := NewHandler(cusdecSvc, cdnSvc)

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
				"eventType": "CDN_INTEGRATED",
				"processedAt": "2026-07-23T10:00:00Z",
				"payload": { "edgeId": "", "integrated": true }
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
			handler := NewHandler(cusdecSvc, cdnSvc)

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
		handler := NewHandler(cusdecSvc, cdnSvc)

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
		handler := NewHandler(cusdecSvc, cdnSvc)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(`{invalid-json`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleWebhook(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Workflow not found by edgeId (404 Not Found)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc)

		payload := `{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-07-23T10:00:00Z",
			"payload": {
				"edgeId": "edge-missing",
				"integrated": true,
				"totalAssessedAmount": 1244,
				"amountPayable": 1244,
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

	t.Run("Integration result before its submission was recorded (503 Service Unavailable)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc)
		cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrDeclarationNotRecorded)
		cdnSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotRecorded)

		for name, payload := range map[string]string{
			"CusDec": `{"eventType": "CUSDEC_INTEGRATED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
				"edgeId": "edge-early", "integrated": true, "totalAssessedAmount": 1244, "amountPayable": 1244,
				"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}}`,
			"CDN": `{"eventType": "CDN_INTEGRATED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
				"edgeId": "edge-early", "integrated": true,
				"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 28237}}}`,
		} {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleWebhook(w, req)

			assert.Equal(t, http.StatusServiceUnavailable, w.Code, name)
		}
	})

	t.Run("CDN callback before its task was waiting (503 Service Unavailable)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc)
		cdnSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cdn.ErrTaskNotParked)
		cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrTaskNotParked)

		for name, payload := range map[string]string{
			"integration result": `{"eventType": "CDN_INTEGRATED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
				"edgeId": "edge-early", "integrated": true,
				"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 28237}}}`,
			"acknowledgment": `{"eventType": "CDN_ACKNOWLEDGED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
				"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 28237}}}`,
		} {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleWebhook(w, req)

			assert.Equal(t, http.StatusServiceUnavailable, w.Code, name)
		}
	})

	t.Run("Cusdec not found by reference (503 Service Unavailable)", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cdnSvc := new(mockCDNService)
		handler := NewHandler(cusdecSvc, cdnSvc)

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
		handler := NewHandler(cusdecSvc, cdnSvc)

		payload := `{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-07-23T10:00:00Z",
			"payload": {
				"edgeId": "edge-err",
				"integrated": true,
				"totalAssessedAmount": 1244,
				"amountPayable": 1244,
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

// A success callback without amountPayable is refused at the boundary: read
// as owing nothing, it would skip the payment step.
func TestSLCEHandler_CusdecIntegratedWithoutAmountPayableIsRejected(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	handler := NewHandler(cusdecSvc, new(mockCDNService))

	payload := `{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-07-23T11:00:00Z",
		"payload": {
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": true,
			"totalAssessedAmount": 1350,
			"cusdecRef": { "year": "2026", "office": "CMB", "serial": "C", "number": 1001 },
			"errors": {}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	cusdecSvc.AssertNotCalled(t, "ProcessIntegrationResult", mock.Anything, mock.Anything)
}

// A rejection whose duties is written as an empty array still reaches the
// service, so the trader hears about it.
func TestSLCEHandler_CusdecRejectionWithEmptyDutiesArrayIsProcessed(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	handler := NewHandler(cusdecSvc, new(mockCDNService))

	payload := `{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-07-23T11:00:00Z",
		"payload": {
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": false,
			"duties": [],
			"errors": { "0": [ { "code": 410, "description": "Missing HS code" } ] }
		}
	}`
	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecIntegrationResultRequest) bool {
		return r.EdgeID == "5516e4c8-a93d-429d-8a18-6a484d331176" && !r.Integrated
	})).Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	cusdecSvc.AssertExpectations(t)
}

// amendedPayload is the §6.5.4 example, with a declaration abridged to a few
// Annex A fields.
const amendedPayload = `{
	"eventType": "CUSDEC_AMENDED",
	"processedAt": "2026-04-26T11:15:22Z",
	"payload": {
		"cusDecRef": {"office": "CBEX1", "year": "2026", "serial": "E", "number": 1047},
		"cusDec": {"properties": {"submitter": "", "nswId": ""}, "baseGeneralSegment": {"declarationType": "EX"}},
		"duties": {
			"globalDuties": [{"typeCode": "EPF", "taxBaseAmount": 550, "taxRateNumeric": 2.0, "taxAssessedAmount": 1100, "paymentMethodCode": "1"}],
			"itemDutiesList": [{"itemSequenceNumeric": 1, "dutyTaxFees": [{"typeCode": "CED", "taxBaseAmount": 125, "taxRateNumeric": 0.0, "taxAssessedAmount": 0, "paymentMethodCode": "1"}]}]
		},
		"operationTime": "2026-10-14T05:07:38Z"
	}
}`

// A §6.5.4 Declaration Amended notification reaches the CusDec service with its
// reference, declaration and operation time, and is answered 200.
func TestSLCEHandler_CusdecAmended(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	handler := NewHandler(cusdecSvc, new(mockCDNService))
	cusdecSvc.On("ProcessAmendment", mock.Anything, mock.MatchedBy(func(req cusdec.CusdecAmendedRequest) bool {
		return req.Payload.CusdecRef == cusdec.DocumentReference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 1047} &&
			req.Payload.OperationTime == "2026-10-14T05:07:38Z" &&
			bytes.Contains(req.Payload.Cusdec, []byte(`"declarationType": "EX"`))
	})).Return(nil)

	rec := httptest.NewRecorder()
	handler.HandleWebhook(rec, httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(amendedPayload)))

	assert.Equal(t, http.StatusOK, rec.Code)
	cusdecSvc.AssertExpectations(t)
}

func TestSLCEHandler_CusdecAmendedResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		err  error
		want int
	}{
		"no declaration object":         {body: strings.Replace(amendedPayload, `"cusDec"`, `"other"`, 1), want: http.StatusBadRequest},
		"no operation time":             {body: strings.Replace(amendedPayload, `"operationTime": "2026-10-14T05:07:38Z"`, `"operationTime": ""`, 1), want: http.StatusBadRequest},
		"reference not registered yet":  {body: amendedPayload, err: cusdec.ErrCusdecNotFoundByRef, want: http.StatusServiceUnavailable},
		"declaration without a case":    {body: amendedPayload, err: cusdec.ErrCaseNotRecorded, want: http.StatusNotFound},
		"the flow could not be started": {body: amendedPayload, err: errors.New("boom"), want: http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			cusdecSvc := new(mockCusdecService)
			cusdecSvc.On("ProcessAmendment", mock.Anything, mock.Anything).Return(tc.err)
			rec := httptest.NewRecorder()

			NewHandler(cusdecSvc, new(mockCDNService)).HandleWebhook(rec,
				httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tc.body)))

			assert.Equal(t, tc.want, rec.Code)
		})
	}
}

// cdnAmendedPayload is the §7.6 example, with a dispatch note abridged to a few
// Annex B fields.
const cdnAmendedPayload = `{
	"eventType": "CDN_AMENDED",
	"processedAt": "2026-04-27T14:30:10Z",
	"payload": {
		"cdnRef": {"office": "CBEX1", "year": "2026", "serial": "C", "number": 28237},
		"cdn": {"properties": {"submitter": "", "nswId": ""}, "containerNumber": "MSCU1234567", "cusDecRefs": [{"office": "CBEX1", "year": "2026", "serial": "E", "number": 1047}]},
		"operationTime": "2026-10-14T05:07:38Z"
	}
}`

// A §7.6 CDN Amended notification reaches the CDN service with its reference,
// dispatch note and operation time, and is answered 200.
func TestSLCEHandler_CDNAmended(t *testing.T) {
	cdnSvc := new(mockCDNService)
	handler := NewHandler(new(mockCusdecService), cdnSvc)
	cdnSvc.On("ProcessAmendment", mock.Anything, mock.MatchedBy(func(req cdn.CDNAmendedRequest) bool {
		return req.Payload.CDNRef == cdn.DocumentReference{Office: "CBEX1", Year: "2026", Serial: "C", Number: 28237} &&
			req.Payload.OperationTime == "2026-10-14T05:07:38Z" &&
			bytes.Contains(req.Payload.CDN, []byte(`"containerNumber": "MSCU1234567"`))
	})).Return(nil)

	rec := httptest.NewRecorder()
	handler.HandleWebhook(rec, httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(cdnAmendedPayload)))

	assert.Equal(t, http.StatusOK, rec.Code)
	cdnSvc.AssertExpectations(t)
}

func TestSLCEHandler_CDNAmendedResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		err  error
		want int
	}{
		"no dispatch note object":       {body: strings.Replace(cdnAmendedPayload, `"cdn":`, `"other":`, 1), want: http.StatusBadRequest},
		"no operation time":             {body: strings.Replace(cdnAmendedPayload, `"operationTime": "2026-10-14T05:07:38Z"`, `"operationTime": ""`, 1), want: http.StatusBadRequest},
		"reference not registered yet":  {body: cdnAmendedPayload, err: cdn.ErrDispatchNoteNotFoundByCDNRef, want: http.StatusServiceUnavailable},
		"dispatch note without a case":  {body: cdnAmendedPayload, err: cdn.ErrCaseNotRecorded, want: http.StatusNotFound},
		"the flow could not be started": {body: cdnAmendedPayload, err: errors.New("boom"), want: http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			cdnSvc := new(mockCDNService)
			cdnSvc.On("ProcessAmendment", mock.Anything, mock.Anything).Return(tc.err)
			rec := httptest.NewRecorder()

			NewHandler(new(mockCusdecService), cdnSvc).HandleWebhook(rec,
				httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tc.body)))

			assert.Equal(t, tc.want, rec.Code)
		})
	}
}

const cdnCancelledPayload = `{
	"eventType": "CDN_CANCELLED",
	"processedAt": "2026-04-27T14:30:10Z",
	"payload": {
		"cdnRef": {"office": "CBEX1", "year": "2026", "serial": "C", "number": 28237}
	}
}`

// A §7.7 CDN Cancelled notification reaches the CDN service with its reference,
// and is answered 200.
func TestSLCEHandler_CDNCancelled(t *testing.T) {
	cdnSvc := new(mockCDNService)
	handler := NewHandler(new(mockCusdecService), cdnSvc)
	cdnSvc.On("ProcessCancellation", mock.Anything, mock.MatchedBy(func(req cdn.CDNCancelledRequest) bool {
		return req.Payload.CDNRef == cdn.DocumentReference{Office: "CBEX1", Year: "2026", Serial: "C", Number: 28237}
	})).Return(nil)

	rec := httptest.NewRecorder()
	handler.HandleWebhook(rec, httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(cdnCancelledPayload)))

	assert.Equal(t, http.StatusOK, rec.Code)
	cdnSvc.AssertExpectations(t)
}

func TestSLCEHandler_CDNCancelledResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		err  error
		want int
	}{
		"no reference":                 {body: strings.Replace(cdnCancelledPayload, `"cdnRef":`, `"other":`, 1), want: http.StatusBadRequest},
		"no processing time":           {body: strings.Replace(cdnCancelledPayload, `"processedAt": "2026-04-27T14:30:10Z",`, ``, 1), want: http.StatusBadRequest},
		"reference not registered yet": {body: cdnCancelledPayload, err: cdn.ErrDispatchNoteNotFoundByCDNRef, want: http.StatusServiceUnavailable},
		"wait not parked yet":          {body: cdnCancelledPayload, err: cdn.ErrTaskNotParked, want: http.StatusServiceUnavailable},
		"the wait could not be ended":  {body: cdnCancelledPayload, err: errors.New("boom"), want: http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			cdnSvc := new(mockCDNService)
			cdnSvc.On("ProcessCancellation", mock.Anything, mock.Anything).Return(tc.err)
			rec := httptest.NewRecorder()

			NewHandler(new(mockCusdecService), cdnSvc).HandleWebhook(rec,
				httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(tc.body)))

			assert.Equal(t, tc.want, rec.Code)
		})
	}
}
