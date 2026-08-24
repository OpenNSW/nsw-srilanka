package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/taskflow/store"
)

// stubService stands in for a registered remote service, recording the request it
// receives so a test can assert on what went on the wire rather than on what an
// interpreter returned.
func stubService(t *testing.T) (*remote.Manager, *http.Header) {
	t.Helper()

	var seen http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"SUCCESS"}`))
	}))
	t.Cleanup(server.Close)

	registry := fmt.Sprintf(`{"version":"1.0","services":[{"id":"svc","url":%q}]}`, server.URL)
	path := filepath.Join(t.TempDir(), "services.json")
	require.NoError(t, os.WriteFile(path, []byte(registry), 0o600))

	manager := remote.NewManager()
	require.NoError(t, manager.LoadServices(path))
	return manager, &seen
}

func callContext(inputs map[string]any) pluginContext {
	return pluginContext{
		Context:         context.Background(),
		Record:          &store.TaskRecord{TaskID: "task-1", Data: map[string]any{}},
		Inputs:          inputs,
		OutputNamespace: "svc",
	}
}

func apiCallConfigJSON(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(apiCallConfig{ServiceID: "svc", Path: "/submit", ResultField: "accepted"})
	require.NoError(t, err)
	return raw
}

// keyedInterpreter needs a header resolved from the task inputs — the case
// HeaderInterpreter exists for.
type keyedInterpreter struct{}

func (keyedInterpreter) BuildRequest(map[string]any) remote.Body {
	return remote.JSONBody{V: map[string]any{"document": "x"}}
}

func (keyedInterpreter) BuildHeaders(inputs map[string]any) map[string]string {
	if key, _ := inputs["client_key"].(string); key != "" {
		return map[string]string{"X-Client-Key": key}
	}
	return nil
}

func (keyedInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	return callErr == nil, map[string]any{"status": resp["status"]}
}

// bodyOnlyInterpreter models only a body, as most interpreters do.
type bodyOnlyInterpreter struct{}

func (bodyOnlyInterpreter) BuildRequest(map[string]any) remote.Body {
	return remote.JSONBody{V: map[string]any{"document": "x"}}
}

func (bodyOnlyInterpreter) Interpret(callErr error, _ map[string]any) (bool, map[string]any) {
	return callErr == nil, nil
}

// A header the interpreter resolves per call reaches the service, which is what
// lets an identifier carried by the workflow — rather than configured on the
// service — identify the submission.
func TestAPICallPlugin_HeaderInterpreterSendsPerCallHeaders(t *testing.T) {
	manager, seen := stubService(t)
	plugin := NewAPICallPluginWithInterpreter(manager, keyedInterpreter{})

	ctx := callContext(map[string]any{"client_key": "issued-key-1"})
	require.NoError(t, plugin.Execute(ctx, apiCallConfigJSON(t)))

	assert.Equal(t, "issued-key-1", seen.Get("X-Client-Key"))

	out, ok := ctx.Record.Data["svc"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, out["accepted"])
}

// No value means no header: an interpreter must not invent an identity, and the
// service is the one to say whether it can identify the caller.
func TestAPICallPlugin_NoHeaderWhenTheInputIsAbsent(t *testing.T) {
	manager, seen := stubService(t)
	plugin := NewAPICallPluginWithInterpreter(manager, keyedInterpreter{})

	require.NoError(t, plugin.Execute(callContext(map[string]any{}), apiCallConfigJSON(t)))
	assert.Empty(t, seen.Get("X-Client-Key"))
}

// An interpreter that models only a body is never asked for headers, so adding
// the contract changes nothing for the interpreters that predate it.
func TestAPICallPlugin_BodyOnlyInterpreterIsNotAskedForHeaders(t *testing.T) {
	manager, seen := stubService(t)
	plugin := NewAPICallPluginWithInterpreter(manager, bodyOnlyInterpreter{})

	require.NoError(t, plugin.Execute(callContext(map[string]any{"client_key": "issued-key-1"}), apiCallConfigJSON(t)))
	assert.Empty(t, seen.Get("X-Client-Key"))
}
