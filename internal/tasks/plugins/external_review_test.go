package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
)

const (
	reviewTaskID = "0b6f3f6e-3c1a-4c55-9a1e-6d1f2f3a4b5c"
	reviewStepID = "5a1d7c2e-8f4b-4e3a-b6c9-0d2e1f3a4b5c"
)

// receivedCall is one request the fake review service saw.
type receivedCall struct {
	path string
	body map[string]any
}

// newReviewService starts a fake review service answering every call with status
// and returns a remote.Manager that knows it as "agency", plus the calls it saw.
func newReviewService(t *testing.T, status int) (*remote.Manager, *[]receivedCall) {
	t.Helper()
	var calls []receivedCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		calls = append(calls, receivedCall{path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	services := fmt.Sprintf(`{"version": "1.0", "services": [{"id": "agency", "url": %q, "timeout": "5s"}]}`, srv.URL)
	file := filepath.Join(t.TempDir(), "services.json")
	require.NoError(t, os.WriteFile(file, []byte(services), 0o600))
	mgr := remote.NewManager()
	require.NoError(t, mgr.LoadServices(file))
	return mgr, &calls
}

func reviewContext(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Context: context.Background(),
		Record:  &store.TaskRecord{TaskID: reviewTaskID, ActiveStepID: reviewStepID, RootWorkflowID: "consignment-1"},
		Inputs:  inputs,
	}
}

func ownToken(t *testing.T) string {
	t.Helper()
	token, err := callbacktoken.Encode(reviewTaskID, reviewStepID)
	require.NoError(t, err)
	return token
}

func TestExternalReview_DispatchesWithoutReplyToken(t *testing.T) {
	mgr, calls := newReviewService(t, http.StatusOK)
	p := NewExternalReviewPlugin(mgr, "https://tnsw.example")
	ctx := reviewContext(map[string]any{"submission": map[string]any{"field": "v1"}})

	err := p.Execute(ctx, json.RawMessage(`{"service_id": "agency", "path": "/api/v1/inject", "task_code": "review_v1"}`))

	require.ErrorIs(t, err, ErrSuspended)
	assert.Equal(t, "QUEUED_EXTERNALLY", ctx.Record.State)
	require.Len(t, *calls, 1)
	call := (*calls)[0]
	assert.Equal(t, "/api/v1/inject", call.path)
	assert.Equal(t, reviewTaskID, call.body["taskId"])
	assert.Equal(t, "review_v1", call.body["taskCode"])
	assert.Equal(t, ownToken(t), call.body["callbackToken"])
	assert.Equal(t, map[string]any{"field": "v1"}, call.body["data"])
}

func TestExternalReview_EmptyReplyTokenDispatches(t *testing.T) {
	mgr, calls := newReviewService(t, http.StatusOK)
	p := NewExternalReviewPlugin(mgr, "https://tnsw.example")
	ctx := reviewContext(map[string]any{"replyToken": "", "submission": map[string]any{}})

	err := p.Execute(ctx, json.RawMessage(`{"service_id": "agency", "path": "/api/v1/inject", "reply_command": "submit"}`))

	require.ErrorIs(t, err, ErrSuspended)
	require.Len(t, *calls, 1)
	assert.Equal(t, "/api/v1/inject", (*calls)[0].path)
}

func TestExternalReview_RepliesOnReplyToken(t *testing.T) {
	mgr, calls := newReviewService(t, http.StatusOK)
	p := NewExternalReviewPlugin(mgr, "https://tnsw.example")
	ctx := reviewContext(map[string]any{
		"replyToken": "tok_x",
		"submission": map[string]any{"field": "v2"},
	})

	// No path: a reply always goes to the callbacks route.
	err := p.Execute(ctx, json.RawMessage(`{"service_id": "agency", "reply_command": "submit"}`))

	require.ErrorIs(t, err, ErrSuspended)
	assert.Equal(t, "QUEUED_EXTERNALLY", ctx.Record.State)
	require.Len(t, *calls, 1)
	call := (*calls)[0]
	assert.Equal(t, "/api/v1/callbacks/tok_x", call.path)
	assert.Equal(t, map[string]any{
		"command": "submit",
		"payload": map[string]any{
			"submission":    map[string]any{"field": "v2"},
			"callbackToken": ownToken(t),
		},
	}, call.body)
}

func TestExternalReview_ReplyToStaleStepIsNotRetried(t *testing.T) {
	mgr, _ := newReviewService(t, http.StatusConflict)
	p := NewExternalReviewPlugin(mgr, "https://tnsw.example")

	err := p.Execute(reviewContext(map[string]any{"replyToken": "tok_x"}),
		json.RawMessage(`{"service_id": "agency", "reply_command": "submit"}`))

	// Wrapped as core wraps a plugin error, so the engine still finds it.
	err = fmt.Errorf("plugin execution failed: %w", err)
	var appErr *temporal.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.True(t, appErr.NonRetryable())
	assert.Equal(t, "StaleReplyToken", appErr.Type())
}

func TestExternalReview_FailedReplyIsRetried(t *testing.T) {
	mgr, _ := newReviewService(t, http.StatusServiceUnavailable)
	p := NewExternalReviewPlugin(mgr, "https://tnsw.example")

	err := p.Execute(reviewContext(map[string]any{"replyToken": "tok_x"}),
		json.RawMessage(`{"service_id": "agency", "reply_command": "submit"}`))

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrSuspended)
	var appErr *temporal.ApplicationError
	assert.False(t, errors.As(err, &appErr), "only a 409 stops the retries")
}

func TestExternalReview_ConfigErrors(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		config string
		want   string
	}{
		{"dispatch without path", map[string]any{}, `{"service_id": "agency"}`, "path is required"},
		{"reply without command", map[string]any{"replyToken": "tok_x"}, `{"service_id": "agency"}`, "reply_command is required"},
		{"reply token not a string", map[string]any{"replyToken": 42}, `{"service_id": "agency", "reply_command": "submit"}`, "replyToken must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, calls := newReviewService(t, http.StatusOK)
			p := NewExternalReviewPlugin(mgr, "https://tnsw.example")

			err := p.Execute(reviewContext(tt.inputs), json.RawMessage(tt.config))

			require.ErrorContains(t, err, tt.want)
			assert.Empty(t, *calls)
		})
	}
}
