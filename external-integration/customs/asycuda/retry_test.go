package asycuda

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

// TestMain shortens our own retries, so a test whose callback finds its task
// not parked does not leave a retry sleeping for minutes.
func TestMain(m *testing.M) {
	taskRetryBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	os.Exit(m.Run())
}

var errNotParked = errors.New("not parked")

func notParked(err error) bool { return errors.Is(err, errNotParked) }

// Each attempt returns the next of results; any past the last return its last.
func attempts(results ...error) (func(context.Context) error, *atomic.Int32) {
	var calls atomic.Int32
	return func(context.Context) error {
		n := int(calls.Add(1))
		return results[min(n, len(results))-1]
	}, &calls
}

func TestTaskRetries_RetryUntilTheTaskParks(t *testing.T) {
	retries := newTaskRetries([]time.Duration{0, 0, 0, 0})
	process, calls := attempts(errNotParked, errNotParked, nil)

	retries.schedule(context.Background(), "cb", process, notParked)
	retries.wait()

	assert.Equal(t, int32(3), calls.Load())
}

func TestTaskRetries_StopOnAnotherError(t *testing.T) {
	retries := newTaskRetries([]time.Duration{0, 0, 0, 0})
	process, calls := attempts(errNotParked, errors.New("boom"))

	retries.schedule(context.Background(), "cb", process, notParked)
	retries.wait()

	assert.Equal(t, int32(2), calls.Load())
}

func TestTaskRetries_GiveUpWhenTheBackoffRunsOut(t *testing.T) {
	retries := newTaskRetries([]time.Duration{0, 0, 0})
	process, calls := attempts(errNotParked)

	retries.schedule(context.Background(), "cb", process, notParked)
	retries.wait()

	assert.Equal(t, int32(3), calls.Load())
}

// A redelivery answered 503 while a retry is pending does not start another.
func TestTaskRetries_OneRetryPerCallback(t *testing.T) {
	retries := newTaskRetries([]time.Duration{20 * time.Millisecond})
	process, calls := attempts(nil)

	retries.schedule(context.Background(), "cb", process, notParked)
	retries.schedule(context.Background(), "cb", process, notParked)
	retries.wait()
	assert.Equal(t, int32(1), calls.Load())

	// Once it has finished, the callback can be retried again.
	retries.schedule(context.Background(), "cb", process, notParked)
	retries.wait()
	assert.Equal(t, int32(2), calls.Load())
}

// A retry outlives the request that scheduled it.
func TestTaskRetries_OutliveTheRequest(t *testing.T) {
	retries := newTaskRetries([]time.Duration{0})
	ctx, cancel := context.WithCancel(context.Background())
	var sawCancel atomic.Bool
	cancel()

	retries.schedule(ctx, "cb", func(ctx context.Context) error {
		sawCancel.Store(ctx.Err() != nil)
		return nil
	}, notParked)
	retries.wait()

	assert.False(t, sawCancel.Load())
}

func TestTaskRetries_RecoverAPanic(t *testing.T) {
	retries := newTaskRetries([]time.Duration{0})

	retries.schedule(context.Background(), "cb", func(context.Context) error { panic("boom") }, notParked)
	retries.wait()

	// The key is released, so the callback can be retried again.
	process, calls := attempts(nil)
	retries.schedule(context.Background(), "cb", process, notParked)
	retries.wait()
	assert.Equal(t, int32(1), calls.Load())
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
)

// A CDN callback that finds its task not parked is accepted with a 202 and
// retried by the handler until the task has parked.
func TestSLCEHandler_CDNCallbackRetriedUntilTheTaskParks(t *testing.T) {
	for name, tc := range map[string]struct {
		body   string
		method string
	}{
		"integration result": {cdnIntegratedPayload, "ProcessIntegrationResult"},
		"acknowledgment":     {cdnAcknowledgedPayload, "ProcessAcknowledgment"},
		"cancellation":       {cdnCancelledPayload, "ProcessCancellation"},
	} {
		t.Run(name, func(t *testing.T) {
			cdnSvc := new(mockCDNService)
			cdnSvc.On(tc.method, mock.Anything, mock.Anything).Return(cdn.ErrTaskNotParked).Twice()
			cdnSvc.On(tc.method, mock.Anything, mock.Anything).Return(nil).Once()
			h := NewHandler(new(mockCusdecService), cdnSvc)

			assert.Equal(t, http.StatusAccepted, post(h, tc.body))
			h.retries.wait()

			cdnSvc.AssertNumberOfCalls(t, tc.method, 3)
		})
	}
}

// A CusDec integration result that finds its review task not parked is
// accepted with a 202 and retried by the handler.
func TestSLCEHandler_CusdecIntegrationResultRetriedUntilTheTaskParks(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrTaskNotParked).Once()
	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(nil).Once()
	h := NewHandler(cusdecSvc, new(mockCDNService))

	assert.Equal(t, http.StatusAccepted, post(h, cusdecIntegratedPayload))
	h.retries.wait()

	cusdecSvc.AssertNumberOfCalls(t, "ProcessIntegrationResult", 2)
}

// When a redelivery from SLC Edge completes the task first, ours finds a
// duplicate and stops.
func TestSLCEHandler_RetryStopsOnceSLCEdgeDelivered(t *testing.T) {
	cusdecSvc := new(mockCusdecService)
	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrTaskNotParked).Once()
	cusdecSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cusdec.ErrDuplicateIntegrationResult)
	h := NewHandler(cusdecSvc, new(mockCDNService))

	assert.Equal(t, http.StatusAccepted, post(h, cusdecIntegratedPayload))
	h.retries.wait()

	cusdecSvc.AssertNumberOfCalls(t, "ProcessIntegrationResult", 2)
}

// Other 503s (the record itself not there yet) are left to SLC Edge's retries.
func TestSLCEHandler_NoRetryWhenTheRecordIsMissing(t *testing.T) {
	cdnSvc := new(mockCDNService)
	cdnSvc.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(cdn.ErrDispatchNoteNotRecorded)
	h := NewHandler(new(mockCusdecService), cdnSvc)

	assert.Equal(t, http.StatusServiceUnavailable, post(h, cdnIntegratedPayload))
	h.retries.wait()

	cdnSvc.AssertNumberOfCalls(t, "ProcessIntegrationResult", 1)
}
