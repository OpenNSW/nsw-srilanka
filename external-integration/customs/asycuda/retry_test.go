package asycuda

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

// fakeScheduler records the callbacks the handler holds for retry.
type fakeScheduler struct {
	held []Callback
}

func (f *fakeScheduler) schedule(_ context.Context, cb Callback) {
	f.held = append(f.held, cb)
}

// withHeld returns h holding its early callbacks in a fakeScheduler.
func withHeld(h *Handler) (*Handler, *fakeScheduler) {
	held := &fakeScheduler{}
	h.callbacks = held
	return h, held
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
			h, held := withHeld(NewHandler(cusdecSvc, cdnSvc))

			assert.Equal(t, http.StatusAccepted, post(h, tc.body))
			require.Len(t, held.held, 1)
			assert.Equal(t, tc.event, held.held[0].EventType)
			assert.JSONEq(t, tc.body, string(held.held[0].Body))
		})
	}
}

// Without retries configured, an early callback is answered 503 as before, for
// SLC Edge to retry.
func TestSLCEHandler_EarlyCallbackWithoutRetries(t *testing.T) {
	cdnSvc := new(mockCDNService)
	cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef)

	assert.Equal(t, http.StatusServiceUnavailable, post(NewHandler(new(mockCusdecService), cdnSvc), cdnAcknowledgedPayload))
}

// A callback that failed for another reason is answered as before, not held.
func TestSLCEHandler_OtherErrorsNotHeld(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrWorkflowNotFoundByEdgeID)
	h, held := withHeld(NewHandler(cusdecSvc, new(mockCDNService)))

	assert.Equal(t, http.StatusNotFound, post(h, cusdecIntegratedPayload))
	assert.Empty(t, held.held)
}

// The handler's retries apply a held callback through its services.
func TestSLCEHandler_HeldCallbackApplied(t *testing.T) {
	cdnSvc := new(mockCDNService)
	cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef).Once()
	cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(nil).Once()
	h := NewHandler(new(mockCusdecService), cdnSvc)
	retries := newCallbackRetries(h.applyCallback, timing(time.Minute))
	h.callbacks = retries

	assert.Equal(t, http.StatusAccepted, post(h, cdnAcknowledgedPayload))
	retries.wait()

	cdnSvc.AssertNumberOfCalls(t, "ProcessAcknowledgment", 2)
}

// A held callback is applied through the service the handler would have used.
// One already applied counts as done; one a retry cannot fix is permanent.
func TestApplyCallback(t *testing.T) {
	t.Run("applied", func(t *testing.T) {
		cdnSvc := new(mockCDNService)
		cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(nil)
		h := NewHandler(new(mockCusdecService), cdnSvc)

		require.NoError(t, h.applyCallback(context.Background(), "CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)))
	})

	t.Run("event takes its type from the callback", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cusdecSvc.On("ProcessEvent", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecEventRequest) bool {
			return r.Event == "PAYMENT_CONFIRMED"
		})).Return(nil)
		h := NewHandler(cusdecSvc, new(mockCDNService))

		require.NoError(t, h.applyCallback(context.Background(), "PAYMENT_CONFIRMED", []byte(paymentConfirmedPayload)))
	})

	t.Run("duplicates are done", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrDuplicateIntegrationResult)
		cusdecSvc.On("ProcessEvent", mock.Anything, mock.Anything).Return(cusdec.ErrDuplicateEvent)
		h := NewHandler(cusdecSvc, new(mockCDNService))

		require.NoError(t, h.applyCallback(context.Background(), "CUSDEC_INTEGRATED", []byte(cusdecIntegratedPayload)))
		require.NoError(t, h.applyCallback(context.Background(), "PAYMENT_CONFIRMED", []byte(paymentConfirmedPayload)))
	})

	t.Run("still early is retried", func(t *testing.T) {
		cdnSvc := new(mockCDNService)
		cdnSvc.On("ProcessAcknowledgment", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotFoundByCDNRef)
		h := NewHandler(new(mockCusdecService), cdnSvc)

		err := h.applyCallback(context.Background(), "CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload))
		require.ErrorIs(t, err, cdn.ErrDispatchNoteNotFoundByCDNRef)
		assert.False(t, isPermanent(err))
	})

	for name, cb := range map[string]Callback{
		"unknown event": {"NOT_AN_EVENT", []byte(`{}`)},
		"invalid body":  {"CDN_ACKNOWLEDGED", []byte(`{broken`)},
	} {
		t.Run(name+" is permanent", func(t *testing.T) {
			h := NewHandler(new(mockCusdecService), new(mockCDNService))
			assert.True(t, isPermanent(h.applyCallback(context.Background(), cb.EventType, cb.Body)))
		})
	}

	t.Run("workflow gone is permanent", func(t *testing.T) {
		cusdecSvc := new(mockCusdecService)
		cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrWorkflowNotFoundByEdgeID)
		h := NewHandler(cusdecSvc, new(mockCDNService))

		assert.True(t, isPermanent(h.applyCallback(context.Background(), "CUSDEC_INTEGRATED", []byte(cusdecIntegratedPayload))))
	})
}

func timing(window time.Duration) callbackRetryTiming {
	return callbackRetryTiming{Initial: time.Millisecond, Max: 4 * time.Millisecond, Window: window}
}

// Each attempt returns the next of results; any past the last return its last.
func attempts(results ...error) (func(context.Context, string, []byte) error, *atomic.Int32) {
	var calls atomic.Int32
	return func(context.Context, string, []byte) error {
		n := int(calls.Add(1))
		return results[min(n, len(results))-1]
	}, &calls
}

var errEarly = cdn.ErrDispatchNoteNotFoundByCDNRef

// A held callback is retried while it is still early, until it lands.
func TestCallbackRetries_RetryUntilApplied(t *testing.T) {
	apply, calls := attempts(errEarly, errEarly, nil)
	retries := newCallbackRetries(apply, timing(time.Minute))

	retries.schedule(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.wait()

	assert.Equal(t, int32(3), calls.Load())
}

// A failed read is retried like an early callback.
func TestCallbackRetries_RetryOtherErrors(t *testing.T) {
	apply, calls := attempts(errors.New("database down"), nil)
	retries := newCallbackRetries(apply, timing(time.Minute))

	retries.schedule(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.wait()

	assert.Equal(t, int32(2), calls.Load())
}

// A callback a retry cannot fix is given up at once.
func TestCallbackRetries_StopOnAPermanentError(t *testing.T) {
	apply, calls := attempts(errEarly, cdn.ErrDispatchNoteNotFoundByEdgeID)
	retries := newCallbackRetries(apply, timing(time.Minute))

	retries.schedule(context.Background(), Callback{"CDN_INTEGRATED", []byte(cdnIntegratedPayload)})
	retries.wait()

	assert.Equal(t, int32(2), calls.Load())
}

// A callback that stays early is given up once the window has passed.
func TestCallbackRetries_GiveUpAfterTheWindow(t *testing.T) {
	apply, calls := attempts(errEarly)
	retries := newCallbackRetries(apply, timing(30*time.Millisecond))

	retries.schedule(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.wait()

	// 1ms, then every 2 to 4ms, inside 30ms.
	assert.Greater(t, calls.Load(), int32(3))
	assert.Less(t, calls.Load(), int32(30))
}

// A redelivery while a callback is held joins its retry; a different callback
// is retried on its own.
func TestCallbackRetries_OneRetryPerCallback(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	retries := newCallbackRetries(func(context.Context, string, []byte) error {
		calls.Add(1)
		<-release
		return nil
	}, timing(time.Minute))

	retries.schedule(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.schedule(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.schedule(context.Background(), Callback{"CDN_CANCELLED", []byte(cdnCancelledPayload)})
	close(release)
	retries.wait()

	assert.Equal(t, int32(2), calls.Load())
}

// A retry outlives the request that held it.
func TestCallbackRetries_OutliveTheRequest(t *testing.T) {
	var sawCancel atomic.Bool
	retries := newCallbackRetries(func(ctx context.Context, _ string, _ []byte) error {
		sawCancel.Store(ctx.Err() != nil)
		return nil
	}, timing(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	retries.schedule(ctx, Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.wait()

	assert.False(t, sawCancel.Load())
}

// A panic in a retry is recovered and releases the callback.
func TestCallbackRetries_RecoverAPanic(t *testing.T) {
	var calls atomic.Int32
	retries := newCallbackRetries(func(context.Context, string, []byte) error {
		if calls.Add(1) == 1 {
			panic("boom")
		}
		return nil
	}, timing(time.Minute))

	retries.schedule(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.wait()
	retries.schedule(context.Background(), Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)})
	retries.wait()

	assert.Equal(t, int32(2), calls.Load())
}

// Each held callback reports how it ended once: the attempts it took, and nil
// once applied or the error it was given up on.
func TestCallbackRetries_ReportTheOutcome(t *testing.T) {
	panics := func(context.Context, string, []byte) error { panic("boom") }
	for name, tc := range map[string]struct {
		apply         func(context.Context, string, []byte) error
		window        time.Duration
		wantErr       error
		wantPermanent bool
		minAttempts   int
		maxAttempts   int
	}{
		"applied":               {apply: first(attempts(errEarly, nil)), window: time.Minute, minAttempts: 2, maxAttempts: 2},
		"refused":               {apply: first(attempts(errEarly, cdn.ErrDispatchNoteNotFoundByEdgeID)), window: time.Minute, wantErr: cdn.ErrDispatchNoteNotFoundByEdgeID, wantPermanent: true, minAttempts: 2, maxAttempts: 2},
		"given up":              {apply: first(attempts(errEarly)), window: 30 * time.Millisecond, wantErr: errEarly, minAttempts: 4, maxAttempts: 29},
		"panicked":              {apply: panics, window: time.Minute, wantErr: errRetryPanicked, wantPermanent: true, minAttempts: 1, maxAttempts: 1},
		"read failed then lost": {apply: first(attempts(errors.New("database down"), cdn.ErrCaseNotRecorded)), window: time.Minute, wantErr: cdn.ErrCaseNotRecorded, wantPermanent: true, minAttempts: 2, maxAttempts: 2},
	} {
		t.Run(name, func(t *testing.T) {
			var outcomes []struct {
				cb       Callback
				attempts int
				err      error
			}
			retries := newCallbackRetries(tc.apply, timing(tc.window))
			retries.outcome = func(_ context.Context, cb Callback, attempts int, err error) {
				outcomes = append(outcomes, struct {
					cb       Callback
					attempts int
					err      error
				}{cb, attempts, err})
			}
			held := Callback{"CDN_ACKNOWLEDGED", []byte(cdnAcknowledgedPayload)}

			retries.schedule(context.Background(), held)
			retries.wait()

			require.Len(t, outcomes, 1)
			assert.Equal(t, held, outcomes[0].cb)
			assert.GreaterOrEqual(t, outcomes[0].attempts, tc.minAttempts)
			assert.LessOrEqual(t, outcomes[0].attempts, tc.maxAttempts)
			if tc.wantErr == nil {
				assert.NoError(t, outcomes[0].err)
			} else {
				assert.ErrorIs(t, outcomes[0].err, tc.wantErr)
				assert.Equal(t, tc.wantPermanent, isPermanent(outcomes[0].err))
			}
		})
	}
}

// first drops the call counter attempts returns alongside its function.
func first(apply func(context.Context, string, []byte) error, _ *atomic.Int32) func(context.Context, string, []byte) error {
	return apply
}
