package trade

import (
	"encoding/json"

	flowplugins "github.com/OpenNSW/nsw-task-flow/plugins"
)

// TODO: github.com/OpenNSW/core is consolidating nsw-task-flow and
// go-temporal-workflow into core/taskflow (with its own TaskPlugin/
// PluginContext types), and this is generic engine-adapter code rather than
// anything Sri Lanka-specific. Once nsw-srilanka migrates its dependency from
// nsw-task-flow/go-temporal-workflow to core, move this file to
// core/taskflow/plugins (rebuilt against core's TaskPlugin/PluginContext) and
// delete this copy.

// ExecutorFunc is synchronous computation logic that can be wrapped as a
// flowplugins.TaskPlugin via NewGenericExecutorPlugin. It has the same shape
// as TaskPlugin.Execute, so any plain function can be registered without
// writing a dedicated plugin struct.
type ExecutorFunc func(ctx flowplugins.PluginContext, config json.RawMessage) error

// genericExecutorPlugin adapts an ExecutorFunc to flowplugins.TaskPlugin —
// the function-to-interface adapter pattern (cf. http.HandlerFunc).
type genericExecutorPlugin struct {
	fn ExecutorFunc
}

// NewGenericExecutorPlugin wraps fn as a TaskPlugin so it can be registered
// under any task type, e.g.:
//
//	pluginsRegistry.Register("HSCODE_SPLIT_BUILDER", trade.NewGenericExecutorPlugin(trade.HscodeSplitBuilderFunc))
func NewGenericExecutorPlugin(fn ExecutorFunc) flowplugins.TaskPlugin {
	return &genericExecutorPlugin{fn: fn}
}

func (p *genericExecutorPlugin) Execute(ctx flowplugins.PluginContext, config json.RawMessage) error {
	return p.fn(ctx, config)
}
