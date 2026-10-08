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
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokenFor stands in for file tokens: "<client>/<value>".
var tokenFor = OGAFiles{
	ClientFor: func(serviceID string) (string, bool) {
		clientID, ok := map[string]string{"cda": "CDA_TO_NSW"}[serviceID]
		return clientID, ok
	},
	IssueForClient: func(clientID, value string) (string, error) {
		return clientID + "/" + value, nil
	},
}

const reviewRenderConfig = `{
  "id": "x:render",
  "files": ["submission.invoice", "submission.documents[*].file", "review.certificate"]
}`

func reviewInputs(t *testing.T) map[string]any {
	t.Helper()
	var in map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"submission": {
			"invoice": "k-invoice",
			"documents": [{"file": "k-doc", "name": "packing list"}],
			"exporter": "ACME"
		}
	}`), &in))
	return in
}

func TestOGAFiles_TokenizeSendsTokens(t *testing.T) {
	in := reviewInputs(t)

	sent, err := tokenFor.tokenize(json.RawMessage(reviewRenderConfig), "submission", in["submission"], "cda")
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"invoice":   "CDA_TO_NSW/k-invoice",
		"documents": []any{map[string]any{"file": "CDA_TO_NSW/k-doc", "name": "packing list"}},
		"exporter":  "ACME",
	}, sent)
	assert.Equal(t, reviewInputs(t), in, "what is sent must be a copy: the task keeps its stored values")
}

func TestOGAFiles_TokenizeWholeInputs(t *testing.T) {
	in := reviewInputs(t)

	sent, err := tokenFor.tokenize(json.RawMessage(reviewRenderConfig), "", in, "cda")
	require.NoError(t, err)
	assert.Equal(t, "CDA_TO_NSW/k-invoice", sent.(map[string]any)["submission"].(map[string]any)["invoice"])
}

// With no client to issue a token to, a file is not sent at all.
func TestOGAFiles_TokenizeFailsClosedWithoutAClient(t *testing.T) {
	_, err := tokenFor.tokenize(json.RawMessage(reviewRenderConfig), "submission", reviewInputs(t)["submission"], "customs")
	assert.ErrorContains(t, err, `"customs"`)
}

func TestOGAFiles_TokenizeWithoutFilesToSend(t *testing.T) {
	in := reviewInputs(t)
	for name, renderConfig := range map[string]string{
		"no files declared":        `{"id":"x:render"}`,
		"none in what is sent":     `{"files":["review.certificate"]}`,
		"declared but not present": `{"files":["submission.missing"]}`,
	} {
		// "customs" has no client: nothing to issue means no client needed.
		sent, err := tokenFor.tokenize(json.RawMessage(renderConfig), "submission", in["submission"], "customs")
		require.NoError(t, err, name)
		assert.Equal(t, in["submission"], sent, name)
	}
}

func TestOGAFiles_TokenizeErrors(t *testing.T) {
	_, err := tokenFor.tokenize(json.RawMessage(`{"files":["submission.docs[0]"]}`), "submission", map[string]any{}, "cda")
	assert.Error(t, err, "a bad declaration")

	failing := tokenFor
	failing.IssueForClient = func(string, string) (string, error) { return "", errors.New("no keyset") }
	_, err = failing.tokenize(json.RawMessage(reviewRenderConfig), "submission", reviewInputs(t)["submission"], "cda")
	assert.ErrorContains(t, err, "no keyset")
}

// newOGA stands up an OGA portal as the outbound service "cda", recording the
// body of each push.
func newOGA(t *testing.T) (*remote.Manager, *[]map[string]any) {
	t.Helper()
	var pushed []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		pushed = append(pushed, body)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "services.json")
	registry := fmt.Sprintf(`{"version":"1","services":[{"id":"cda","url":%q}]}`, srv.URL)
	require.NoError(t, os.WriteFile(path, []byte(registry), 0o600))
	m := remote.NewManager()
	require.NoError(t, m.LoadServices(path))
	return m, &pushed
}

func TestExternalReview_PushesFileTokens(t *testing.T) {
	m, pushed := newOGA(t)
	p := NewExternalReviewPlugin(m, "http://nsw.local", tokenFor)
	in := reviewInputs(t)
	record := &store.TaskRecord{
		TaskID:       uuid.NewString(),
		ActiveStepID: uuid.NewString(),
		RenderConfig: json.RawMessage(reviewRenderConfig),
		Data:         reviewInputs(t),
	}

	err := p.Execute(pluginContext{Context: context.Background(), Record: record, Inputs: in}, json.RawMessage(`{"service_id":"cda","path":"/review"}`))

	require.ErrorIs(t, err, ErrSuspended)
	require.Len(t, *pushed, 1)
	data := (*pushed)[0]["data"].(map[string]any)
	assert.Equal(t, "CDA_TO_NSW/k-invoice", data["invoice"])
	assert.Equal(t, reviewInputs(t), record.Data, "the task keeps its stored values")
	assert.Equal(t, reviewInputs(t), in)
}

func TestExternalReview_DoesNotPushFilesWithoutAClient(t *testing.T) {
	m, pushed := newOGA(t)
	p := NewExternalReviewPlugin(m, "http://nsw.local", OGAFiles{
		ClientFor:      func(string) (string, bool) { return "", false },
		IssueForClient: tokenFor.IssueForClient,
	})
	record := &store.TaskRecord{TaskID: uuid.NewString(), ActiveStepID: uuid.NewString(), RenderConfig: json.RawMessage(reviewRenderConfig)}

	err := p.Execute(pluginContext{Context: context.Background(), Record: record, Inputs: reviewInputs(t)}, json.RawMessage(`{"service_id":"cda","path":"/review"}`))

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrSuspended)
	assert.Empty(t, *pushed)
}
