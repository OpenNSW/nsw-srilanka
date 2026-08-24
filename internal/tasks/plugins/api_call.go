package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/core/remote"
)

// Interpreter adapts a domain to the generic API-call plugin: it builds the
// request body from the task inputs and interprets the response into an
// acceptance flag plus fields to record into the output namespace.
type Interpreter interface {
	// BuildRequest returns the request body to POST, derived from the task's
	// mapped inputs — e.g. selecting a payload key and injecting identifiers.
	BuildRequest(inputs map[string]any) remote.Body

	// Interpret turns the call outcome into a result: whether the call was
	// accepted, plus the fields to record into the output namespace (e.g.
	// response identifiers, or an error message on rejection). callErr is the
	// transport/HTTP error (nil on success); resp is the decoded response body,
	// which the remote client populates even on a 4xx/5xx JSON error.
	Interpret(callErr error, resp map[string]any) (accepted bool, captured map[string]any)
}

// MultipartInterpreter is implemented by interpreters whose service expects a
// multipart/form-data body — a JSON document alongside file uploads — rather
// than a JSON body. The plugin routes such an interpreter through the
// multipart transport and never calls BuildRequest for it.
//
// BuildParts takes a context because assembling the body may need to fetch the
// attachments from storage. An error is passed to Interpret as callErr, so a
// failure to assemble the request is reported through the same path as a
// failure to send it.
type MultipartInterpreter interface {
	Interpreter

	BuildParts(ctx context.Context, inputs map[string]any) ([]remote.Part, error)
}

// passthroughInterpreter sends the "payload" input as-is and treats any
// transport-level success as accepted.
type passthroughInterpreter struct{}

func (passthroughInterpreter) BuildRequest(inputs map[string]any) remote.Body {
	if v, ok := inputs["payload"]; ok {
		return remote.JSONBody{V: v}
	}
	return remote.JSONBody{V: inputs}
}

func (passthroughInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	if callErr != nil {
		return false, map[string]any{"error": callErr.Error()}
	}
	out := map[string]any{}
	if resp != nil {
		out["response"] = resp
	}
	return true, out
}

// APICallPlugin makes an authenticated POST to a configured service and records
// the outcome. The request body and response interpretation are delegated to an
// Interpreter, so the plugin itself is domain-agnostic — any request/response
// shape works.
type APICallPlugin struct {
	manager     *remote.Manager
	interpreter Interpreter
}

func NewAPICallPlugin(manager *remote.Manager) *APICallPlugin {
	return &APICallPlugin{manager: manager, interpreter: passthroughInterpreter{}}
}

func NewAPICallPluginWithInterpreter(manager *remote.Manager, interp Interpreter) *APICallPlugin {
	p := NewAPICallPlugin(manager)
	if interp != nil {
		p.interpreter = interp
	}
	return p
}

type apiCallConfig struct {
	ServiceID   string `json:"service_id"`
	Path        string `json:"path"`
	ResultField string `json:"result_field,omitempty"` // record the accepted flag under this key
}

func (p *APICallPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg apiCallConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("api_call: invalid config: %w", err)
	}
	if cfg.ServiceID == "" || cfg.Path == "" {
		return fmt.Errorf("api_call: service_id and path are required")
	}

	ctx.Record.State = "DISPATCHED"

	var resp map[string]any
	var callErr error
	if mp, ok := p.interpreter.(MultipartInterpreter); ok {
		callErr = p.callMultipart(ctx, mp, cfg, &resp)
	} else {
		req := remote.Request{Method: "POST", Path: cfg.Path, Body: p.interpreter.BuildRequest(ctx.Inputs)}
		callErr = p.manager.Call(ctx.Context, cfg.ServiceID, req, &resp)
	}

	accepted, out := p.interpreter.Interpret(callErr, resp)
	if out == nil {
		out = map[string]any{}
	}
	if cfg.ResultField != "" {
		out[cfg.ResultField] = accepted
	}
	if ns := ctx.OutputNamespace; ns != "" {
		ctx.Record.Data[ns] = out
	}

	if accepted {
		slog.Info("api_call: request accepted", "taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID)
	} else {
		slog.Warn("api_call: request not accepted", "taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID, "callErr", callErr, "result", out)
	}
	return nil
}

// callMultipart assembles and sends a multipart/form-data submission. A
// failure to assemble the body is returned rather than sent, so the
// interpreter reports it to the trader through the same path as a rejection —
// nothing reaches the service in that case.
func (p *APICallPlugin) callMultipart(ctx pluginContext, mp MultipartInterpreter, cfg apiCallConfig, resp *map[string]any) error {
	parts, err := mp.BuildParts(ctx.Context, ctx.Inputs)
	if err != nil {
		return err
	}
	return p.manager.Call(ctx.Context, cfg.ServiceID, remote.Request{
		Method: "POST",
		Path:   cfg.Path,
		Body:   remote.MultipartBody{Parts: parts},
	}, resp)
}
