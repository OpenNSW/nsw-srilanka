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
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

// fakeScheduler records the callbacks the handler holds for retry.
type fakeScheduler struct {
	held []Callback
	err  error
}

func (f *fakeScheduler) Schedule(_ context.Context, cb Callback) error {
	if f.err != nil {
		return f.err
	}
	f.held = append(f.held, cb)
	return nil
}

func post(h *Handler, body string) int {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/slce", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)
	return w.Code
}

const (
	cdnIntegratedPayload = `{"eventType": "CDN_INTEGRATED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
		"edgeId": "edge-early", "integrated": true,
		"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 28237}}}`
	cdnAcknowledgedPayload = `{"eventType": "CDN_ACKNOWLEDGED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
		"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "C", "number": 28237}}}`
	cusdecIntegratedPayload = `{"eventType": "CUSDEC_INTEGRATED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
		"edgeId": "edge-early", "integrated": true, "totalAssessedAmount": 1244, "amountPayable": 1244,
		"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}}`
	paymentConfirmedPayload = `{"eventType": "PAYMENT_CONFIRMED", "processedAt": "2026-07-23T10:00:00Z", "payload": {
		"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}}}`
)

// A callback that arrives before what it answers is there is accepted with a
// 202 and held for retry, whichever early condition it met.
func TestSLCEHandler_EarlyCallbackHeldForRetry(t *testing.T) {
	for name, tc := range map[string]struct {
		event, body string
		setup       func(*mockCusdecService, *mockCDNService)
	}{
		"CusDec result, declaration not recorded": {"CUSDEC_INTEGRATED", cusdecIntegratedPayload, func(c *mockCusdecService, _ *mockCDNService) {
			c.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrDeclarationNotRecorded)
		}},
		"CusDec result, task not parked": {"CUSDEC_INTEGRATED", cusdecIntegratedPayload, func(c *mockCusdecService, _ *mockCDNService) {
			c.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrTaskNotParked)
		}},
		"payment before the integration result": {"PAYMENT_CONFIRMED", paymentConfirmedPayload, func(c *mockCusdecService, _ *mockCDNService) {
			c.On("ProcessEvent", mock.Anything, mock.Anything).Return(cusdec.ErrCusdecNotFoundByRef)
		}},
		"CusDec cancellation before the integration result": {cusdec.EventCusdecCancelled, cancelledPayload, func(c *mockCusdecService, _ *mockCDNService) {
			c.On("ProcessCancellation", mock.Anything, mock.Anything).Return(cusdec.ErrCusdecNotFoundByRef)
		}},
		"CDN result, note not recorded": {"CDN_INTEGRATED", cdnIntegratedPayload, func(_ *mockCusdecService, n *mockCDNService) {
			n.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotRecorded)
		}},
		"CDN acknowledgment before the integration result": {"CDN_ACKNOWLEDGED", cdnAcknowledgedPayload, func(_ *mockCusdecService, n *mockCDNService) {
			n.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef)
		}},
		"CDN cancellation, wait not parked": {cdn.EventCDNCancelled, cdnCancelledPayload, func(_ *mockCusdecService, n *mockCDNService) {
			n.On("ProcessCancellation", mock.Anything, mock.Anything).Return(cdn.ErrTaskNotParked)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			cusdecSvc, cdnSvc := new(mockCusdecService), new(mockCDNService)
			tc.setup(cusdecSvc, cdnSvc)
			held := &fakeScheduler{}
			h := NewHandler(cusdecSvc, cdnSvc).WithCallbackRetries(held)

			assert.Equal(t, http.StatusAccepted, post(h, tc.body))
			require.Len(t, held.held, 1)
			assert.Equal(t, tc.event, held.held[0].EventType)
			assert.JSONEq(t, tc.body, string(held.held[0].Body))
		})
	}
}

// When the retry cannot be started, the callback is answered 503 so that SLC
// Edge retries it: it is never acknowledged without being held.
func TestSLCEHandler_EarlyCallbackNotHeldAsksSLCEdgeToRetry(t *testing.T) {
	cdnSvc := new(mockCDNService)
	cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef)
	h := NewHandler(new(mockCusdecService), cdnSvc).WithCallbackRetries(&fakeScheduler{err: errors.New("temporal down")})

	assert.Equal(t, http.StatusServiceUnavailable, post(h, cdnAcknowledgedPayload))
}

// A callback that failed for another reason is answered as before, not held.
func TestSLCEHandler_OtherErrorsNotHeld(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrWorkflowNotFoundByEdgeID)
	held := &fakeScheduler{}
	h := NewHandler(cusdecSvc, new(mockCDNService)).WithCallbackRetries(held)

	assert.Equal(t, http.StatusNotFound, post(h, cusdecIntegratedPayload))
	assert.Empty(t, held.held)
}

// A held callback is applied through the service the handler would have used.
// One already applied counts as done; one a retry cannot fix is permanent.
func TestApplyCallbackActivity(t *testing.T) {
	t.Run("applied", func(t *testing.T) {
		cdnSvc := new(mockCDNService)
		cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(nil)
		h := NewHandler(new(mockCusdecService), cdnSvc)

		require.NoError(t, h.applyCallbackActivity(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)}))
	})

	t.Run("event sets the expected type", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
			return r.Event == "PAYMENT_CONFIRMED"
		})).Return(nil)
		h := NewHandler(cusdecSvc, new(mockCDNService))

		require.NoError(t, h.applyCallbackActivity(context.Background(), Callback{"PAYMENT_CONFIRMED", []byte(paymentConfirmedPayload)}))
	})

	t.Run("duplicates are done", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrDuplicateIntegrationResult)
		cusdecSvc.On("ProcessEvent", mock.Anything, mock.Anything).Return(cusdec.ErrDuplicateEvent)
		h := NewHandler(cusdecSvc, new(mockCDNService))

		require.NoError(t, h.applyCallbackActivity(context.Background(), Callback{"CUSDEC_INTEGRATED", []byte(cusdecIntegratedPayload)}))
		require.NoError(t, h.applyCallbackActivity(context.Background(), Callback{"PAYMENT_CONFIRMED", []byte(paymentConfirmedPayload)}))
	})

	t.Run("still early is retried", func(t *testing.T) {
		cdnSvc := new(mockCDNService)
		cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef)
		h := NewHandler(new(mockCusdecService), cdnSvc)

		err := h.applyCallbackActivity(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
		require.ErrorIs(t, err, cdn.ErrDispatchNoteNotFoundByCDNRef)
		assert.False(t, isNonRetryable(err))
	})

	for name, cb := range map[string]Callback{
		"unknown event": {"NOT_AN_EVENT", []byte(`{}`)},
		"invalid body":  {"CDN_ACKNOWLEDGED", []byte(`{broken`)},
	} {
		t.Run(name+" is permanent", func(t *testing.T) {
			h := NewHandler(new(mockCusdecService), new(mockCDNService))
			assert.True(t, isNonRetryable(h.applyCallbackActivity(context.Background(), cb)))
		})
	}

	t.Run("workflow gone is permanent", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrWorkflowNotFoundByEdgeID)
		h := NewHandler(cusdecSvc, new(mockCDNService))

		assert.True(t, isNonRetryable(h.applyCallbackActivity(context.Background(), Callback{"CUSDEC_INTEGRATED", []byte(cusdecIntegratedPayload)})))
	})
}

func isNonRetryable(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.NonRetryable()
}

func callbackEnv(t *testing.T, h *Handler) *testsuite.TestWorkflowEnvironment {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(callbackWorkflow)
	env.RegisterActivityWithOptions(h.applyCallbackActivity, activityRegisterOptions())
	t.Cleanup(func() { env.AssertExpectations(t) })
	return env
}

// The workflow retries a callback that is still early until it lands.
func TestCallbackWorkflow_RetriesUntilApplied(t *testing.T) {
	cdnSvc := new(mockCDNService)
	cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef).Twice()
	cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(nil).Once()
	env := callbackEnv(t, NewHandler(new(mockCusdecService), cdnSvc))

	env.ExecuteWorkflow(callbackWorkflow, Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	cdnSvc.AssertNumberOfCalls(t, "ProcessAcknowledgment", 3)
}

// A callback a retry cannot fix is given up at once.
func TestCallbackWorkflow_StopsOnAPermanentError(t *testing.T) {
	cdnSvc := new(mockCDNService)
	cdnSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByEdgeID)
	env := callbackEnv(t, NewHandler(new(mockCusdecService), cdnSvc))

	env.ExecuteWorkflow(callbackWorkflow, Callback{"CDN_INTEGRATED", []byte(cdnIntegratedPayload)})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	cdnSvc.AssertNumberOfCalls(t, "ProcessIntegrationResult", 1)
}

// A callback that stays early is given up. A Temporal server retries it until
// callbackRetryWindow runs out; the test environment stops after 10 attempts.
func TestCallbackWorkflow_GivesUpWhenItStaysEarly(t *testing.T) {
	cdnSvc := new(mockCDNService)
	cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef)
	env := callbackEnv(t, NewHandler(new(mockCusdecService), cdnSvc))

	env.ExecuteWorkflow(callbackWorkflow, Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	assert.Contains(t, env.GetWorkflowError().Error(), cdn.ErrDispatchNoteNotFoundByCDNRef.Error())
	assert.Greater(t, len(cdnSvc.Calls), 1)
}
