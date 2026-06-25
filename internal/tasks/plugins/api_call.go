package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/core/remote"
)

// Interpreter adapts a domain to the generic API-call plugin: it builds the
// request body from the task inputs and interprets the response into an
// acceptance flag plus fields to record into the output namespace.
type Interpreter interface {
	BuildRequest(inputs map[string]any) any
	Interpret(callErr error, resp map[string]any) (accepted bool, captured map[string]any)
}

// passthroughInterpreter sends the "payload" input as-is and treats any
// transport-level success as accepted.
type passthroughInterpreter struct{}

func (passthroughInterpreter) BuildRequest(inputs map[string]any) any {
	if v, ok := inputs["payload"]; ok {
		return v
	}
	return inputs
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

	body := p.interpreter.BuildRequest(ctx.Inputs)
	ctx.Record.State = "DISPATCHED"

	var resp map[string]any
	req := remote.Request{Method: "POST", Path: cfg.Path, Body: body}
	callErr := p.manager.Call(ctx.Context, cfg.ServiceID, req, &resp)

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
