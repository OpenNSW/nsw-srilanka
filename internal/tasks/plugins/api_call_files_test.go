package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
)

// decisionFiles issues tokens "<client>/<value>" for the service "agency".
var decisionFiles = OGAFiles{
	ClientFor: func(serviceID string) (string, bool) {
		return "NSW_TO_AGENCY", serviceID == "agency"
	},
	IssueForClient: func(clientID, value string) (string, error) { return clientID + "/" + value, nil },
}

// decisionRender declares the officer's upload where the decision step sends it,
// and where the officer's form and the injected submission show it.
const decisionRender = `{"files":["payload.payload.certificate","reviewform.certificate","submission.invoice"]}`

func decisionInputs() map[string]any {
	return map[string]any{
		"payload":    map[string]any{"command": "approve", "payload": map[string]any{"certificate": "k-cert", "outcome": "ok"}},
		"reviewform": map[string]any{"certificate": "k-cert"},
		"submission": map[string]any{"invoice": "tnsw:tok-invoice"},
	}
}

func runCall(t *testing.T, p *APICallPlugin, renderConfig string, inputs map[string]any) error {
	t.Helper()
	record := &store.TaskRecord{TaskID: uuid.NewString(), Data: map[string]any{}, RenderConfig: json.RawMessage(renderConfig)}
	return p.Execute(pluginContext{Context: context.Background(), Record: record, Inputs: inputs}, json.RawMessage(`{"service_id":"agency","path":"/api/v1/callbacks/tok"}`))
}

// The step sends the files it declares in its payload as tokens for the
// service's client, and nothing else: the inputs that are only for its page keep
// their values, and are not sent.
func TestAPICall_SendsTokensForDeclaredPayloadFiles(t *testing.T) {
	mgr, svc := newReviewService(t, 200)
	in := decisionInputs()

	require.NoError(t, runCall(t, NewAPICallPlugin(mgr, decisionFiles), decisionRender, in))

	require.Len(t, svc.calls(), 1)
	assert.Equal(t, map[string]any{
		"command": "approve",
		"payload": map[string]any{"certificate": "NSW_TO_AGENCY/k-cert", "outcome": "ok"},
	}, svc.calls()[0].body)
	assert.Equal(t, decisionInputs(), in, "the step's inputs are not changed")
}

func TestAPICall_WithoutAPayloadInputSendsAllTheInputsTokenized(t *testing.T) {
	mgr, svc := newReviewService(t, 200)

	require.NoError(t, runCall(t, NewAPICallPlugin(mgr, decisionFiles), `{"files":["reviewform.certificate"]}`,
		map[string]any{"reviewform": map[string]any{"certificate": "k-cert"}}))

	assert.Equal(t, map[string]any{"reviewform": map[string]any{"certificate": "NSW_TO_AGENCY/k-cert"}}, svc.calls()[0].body)
}

// Without issuers, the payload goes as it is.
func TestAPICall_WithoutFileTokensSendsThePayloadAsIs(t *testing.T) {
	mgr, svc := newReviewService(t, 200)

	require.NoError(t, runCall(t, NewAPICallPlugin(mgr, OGAFiles{}), decisionRender, decisionInputs()))

	assert.Equal(t, "k-cert", svc.calls()[0].body["payload"].(map[string]any)["certificate"])
}

// Domain interpreters shape their own request and read files themselves, so
// their steps are not tokenized.
type allInputs struct{ passthroughInterpreter }

func (allInputs) BuildRequest(inputs map[string]any) remote.Body { return remote.JSONBody{V: inputs} }

func TestAPICall_InterpreterStepsAreNotTokenized(t *testing.T) {
	mgr, svc := newReviewService(t, 200)
	p := NewAPICallPluginWithInterpreter(mgr, allInputs{})
	p.files = decisionFiles

	require.NoError(t, runCall(t, p, decisionRender, decisionInputs()))

	assert.Equal(t, "k-cert", svc.calls()[0].body["reviewform"].(map[string]any)["certificate"])
}

// A file that cannot be issued to the service's client is not sent as it is.
func TestAPICall_FailsClosedWhenTokensCannotBeIssued(t *testing.T) {
	t.Run("no client for the service", func(t *testing.T) {
		mgr, svc := newReviewService(t, 200)
		noClient := decisionFiles
		noClient.ClientFor = func(string) (string, bool) { return "", false }

		err := runCall(t, NewAPICallPlugin(mgr, noClient), decisionRender, decisionInputs())

		assert.ErrorContains(t, err, `"agency"`)
		assert.Empty(t, svc.calls())
	})
	t.Run("a token too long for the service is not retried", func(t *testing.T) {
		mgr, svc := newReviewService(t, 200)
		tooLong := decisionFiles
		tooLong.IssueForClient = func(string, string) (string, error) {
			return "", fmt.Errorf("%w: 3000 bytes", fileaccess.ErrTooLong)
		}

		err := runCall(t, NewAPICallPlugin(mgr, tooLong), decisionRender, decisionInputs())

		var appErr *temporal.ApplicationError
		require.True(t, errors.As(err, &appErr), "err = %v", err)
		assert.True(t, appErr.NonRetryable())
		assert.Empty(t, svc.calls())
	})
}
