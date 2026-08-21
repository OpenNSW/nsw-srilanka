package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/core/remote"
)

// ResponseInterpreter reads the outcome of an API call. Every interpreter does
// this, whatever shape its request takes, so it is the contract the plugin holds
// and the one the request-building interfaces below extend.
type ResponseInterpreter interface {
	// Interpret turns the call outcome into a result: whether the call was
	// accepted, plus the fields to record into the output namespace (e.g.
	// response identifiers, or an error message on rejection). callErr is the
	// transport/HTTP error (nil on success); resp is the decoded response body,
	// which the remote client populates even on a 4xx/5xx JSON error.
	Interpret(callErr error, resp map[string]any) (accepted bool, captured map[string]any)
}

// Interpreter is the plain JSON case: a body derived from the task inputs, and
// the response read back.
type Interpreter interface {
	ResponseInterpreter

	// BuildRequest returns the request body to POST, derived from the task's
	// mapped inputs — e.g. selecting a payload key and injecting identifiers.
	BuildRequest(inputs map[string]any) any
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
	ResponseInterpreter

	BuildParts(ctx context.Context, inputs map[string]any) ([]remote.Part, error)
}

// CallInterpreter is implemented by interpreters whose service needs more of the
// request than a JSON body: headers computed per call, and a body that may fail
// to assemble.
//
// The plugin routes such an interpreter through BuildCall and never calls
// BuildRequest for it. Like BuildParts, an error is passed to Interpret as
// callErr, so nothing is sent and the reason reaches the trader through the path
// a rejection already takes.
//
// consignmentID is the task's root workflow id — the consignment the task
// belongs to — which is what an interpreter needs to resolve a per-consignment
// or per-company identifier. It is passed rather than reached for so the
// interpreter stays independent of how tasks are stored.
type CallInterpreter interface {
	ResponseInterpreter

	BuildCall(ctx context.Context, consignmentID string, inputs map[string]any) (body any, headers map[string]string, err error)
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
	interpreter ResponseInterpreter
}

func NewAPICallPlugin(manager *remote.Manager) *APICallPlugin {
	return &APICallPlugin{manager: manager, interpreter: passthroughInterpreter{}}
}

func NewAPICallPluginWithInterpreter(manager *remote.Manager, interp ResponseInterpreter) *APICallPlugin {
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
	switch interp := p.interpreter.(type) {
	case CallInterpreter:
		callErr = p.callWithHeaders(ctx, interp, cfg, &resp)
	case MultipartInterpreter:
		callErr = p.callMultipart(ctx, interp, cfg, &resp)
	case Interpreter:
		req := remote.Request{Method: "POST", Path: cfg.Path, Body: interp.BuildRequest(ctx.Inputs)}
		callErr = p.manager.Call(ctx.Context, cfg.ServiceID, req, &resp)
	default:
		// Registration error rather than a trader-facing one: an interpreter that
		// can read a response but not build a request cannot be called at all.
		return fmt.Errorf("api_call: interpreter %T builds no request body", p.interpreter)
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

// callWithHeaders assembles and sends a request whose headers are resolved per
// call. A failure to assemble is returned rather than sent, so the interpreter
// reports it to the trader through the same path as a rejection — nothing
// reaches the service in that case.
func (p *APICallPlugin) callWithHeaders(ctx pluginContext, ci CallInterpreter, cfg apiCallConfig, resp *map[string]any) error {
	body, headers, err := ci.BuildCall(ctx.Context, ctx.Record.RootWorkflowID, ctx.Inputs)
	if err != nil {
		return err
	}
	return p.manager.Call(ctx.Context, cfg.ServiceID, remote.Request{
		Method:  "POST",
		Path:    cfg.Path,
		Body:    body,
		Headers: headers,
	}, resp)
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
	return p.manager.CallMultipart(ctx.Context, cfg.ServiceID, remote.MultipartRequest{
		Method: "POST",
		Path:   cfg.Path,
		Parts:  parts,
	}, resp)
}
