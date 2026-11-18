package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCallService starts a fake service answering every call with status, and returns
// a remote.Manager that knows it as "agency" plus a count of the calls it received.
func newCallService(t *testing.T, status int) (*remote.Manager, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id": "r-1"}`))
	}))
	t.Cleanup(srv.Close)

	mgr := remote.NewManager()
	require.NoError(t, mgr.SetServices(remote.ServiceConfig{ID: "agency", URL: srv.URL, Timeout: "5s"}))
	return mgr, &calls
}

func apiCallContext() pluginContext {
	return pluginContext{
		Context:         context.Background(),
		Record:          &store.TaskRecord{TaskID: "task-1", CaseID: "case-1", Data: map[string]any{}},
		Inputs:          map[string]any{"payload": map[string]any{"command": "submitted"}},
		OutputNamespace: "notifyresult",
	}
}

const (
	callConfig      = `{"service_id": "agency", "path": "/api/v1/callbacks/t-1", "result_field": "notified"}`
	retryCallConfig = `{"service_id": "agency", "path": "/api/v1/callbacks/t-1", "result_field": "notified", "retry": true}`
)

// Without retry, a failed call completes the step with the failure recorded, for the
// flow to handle.
func TestAPICall_RecordsAFailedCallWithoutRetry(t *testing.T) {
	mgr, calls := newCallService(t, http.StatusServiceUnavailable)
	ctx := apiCallContext()

	err := NewAPICallPlugin(mgr).Execute(ctx, json.RawMessage(callConfig))

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	out, ok := ctx.Record.Data["notifyresult"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, out["notified"])
	assert.NotEmpty(t, out["error"])
}

// With retry, a failure sending again can fix fails the step as retryable, so the
// engine retries it.
func TestAPICall_RetryFailsTheStepOnARetryableStatus(t *testing.T) {
	for _, status := range []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mgr, calls := newCallService(t, status)
			ctx := apiCallContext()

			err := NewAPICallPlugin(mgr).Execute(ctx, json.RawMessage(retryCallConfig))

			require.Error(t, err)
			_, permanent := flowplugins.PermanentType(err)
			assert.False(t, permanent, "the engine retries the step")
			assert.Equal(t, int32(1), calls.Load())
			assert.NotContains(t, ctx.Record.Data, "notifyresult", "a failed attempt records nothing")
		})
	}
}

// With retry, a 4xx that sending again can't change parks the step at once.
func TestAPICall_RetryParksTheStepOnAPermanentStatus(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mgr, _ := newCallService(t, status)
			ctx := apiCallContext()

			err := NewAPICallPlugin(mgr).Execute(ctx, json.RawMessage(retryCallConfig))

			require.Error(t, err)
			errType, permanent := flowplugins.PermanentType(err)
			assert.True(t, permanent)
			assert.Equal(t, flowplugins.ErrorTypeRejected, errType)
			assert.NotContains(t, ctx.Record.Data, "notifyresult")
		})
	}
}

// With retry, a service that can't be reached fails the step as retryable.
func TestAPICall_RetryFailsTheStepWhenTheServiceIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	mgr := remote.NewManager()
	require.NoError(t, mgr.SetServices(remote.ServiceConfig{ID: "agency", URL: srv.URL, Timeout: "5s"}))
	ctx := apiCallContext()

	err := NewAPICallPlugin(mgr).Execute(ctx, json.RawMessage(retryCallConfig))

	require.Error(t, err)
	_, permanent := flowplugins.PermanentType(err)
	assert.False(t, permanent)
	assert.NotContains(t, ctx.Record.Data, "notifyresult")
}

// With retry, a call that succeeds completes the step as it does without it.
func TestAPICall_RetryCompletesTheStepOnSuccess(t *testing.T) {
	mgr, calls := newCallService(t, http.StatusOK)
	ctx := apiCallContext()

	err := NewAPICallPlugin(mgr).Execute(ctx, json.RawMessage(retryCallConfig))

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, map[string]any{
		"notified": true,
		"response": map[string]any{"id": "r-1"},
	}, ctx.Record.Data["notifyresult"])
	assert.Equal(t, "QUEUED_EXTERNALLY", ctx.Record.State)
}

// decliningInterpreter accepts no answer, as a domain interpreter does with a
// refusal in a successful response body.
type decliningInterpreter struct{ passthroughInterpreter }

func (decliningInterpreter) Interpret(error, map[string]any) (bool, map[string]any) {
	return false, map[string]any{"error": "declined"}
}

// A successful call the interpreter does not accept is the service's answer: retry
// leaves it to the flow.
func TestAPICall_RetryRecordsAnAnswerTheInterpreterDeclines(t *testing.T) {
	mgr, _ := newCallService(t, http.StatusOK)
	ctx := apiCallContext()

	err := NewAPICallPluginWithInterpreter(mgr, decliningInterpreter{}).Execute(ctx, json.RawMessage(retryCallConfig))

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"notified": false, "error": "declined"}, ctx.Record.Data["notifyresult"])
}

// failingPartsInterpreter can't assemble its multipart body.
type failingPartsInterpreter struct{ passthroughInterpreter }

func (failingPartsInterpreter) BuildParts(context.Context, map[string]any) ([]remote.Part, error) {
	return nil, errors.New("storage unavailable")
}

// With retry, a multipart body that can't be assembled fails the step as retryable
// without calling the service; without it, the failure is recorded.
func TestAPICall_MultipartAssemblyFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		config  string
		wantErr bool
	}{
		"with retry":    {retryCallConfig, true},
		"without retry": {callConfig, false},
	} {
		t.Run(name, func(t *testing.T) {
			mgr, calls := newCallService(t, http.StatusOK)
			ctx := apiCallContext()

			err := NewAPICallPluginWithInterpreter(mgr, failingPartsInterpreter{}).Execute(ctx, json.RawMessage(tc.config))

			assert.Equal(t, int32(0), calls.Load(), "nothing is sent")
			if tc.wantErr {
				require.Error(t, err)
				_, permanent := flowplugins.PermanentType(err)
				assert.False(t, permanent)
				assert.ErrorContains(t, err, "storage unavailable")
				return
			}
			require.NoError(t, err)
			out, ok := ctx.Record.Data["notifyresult"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, false, out["notified"])
		})
	}
}
