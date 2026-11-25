package plugins

import (
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
)

type pluginContext = flowplugins.PluginContext

// configError marks err as a configuration error retrying can't fix, so the step
// parks for an admin at once (see flowplugins.Permanent).
func configError(err error) error {
	return flowplugins.Permanent(flowplugins.ErrorTypeConfig, err)
}

// inputError marks err as an error in the step's data that retrying can't fix:
// the inputs are the same on every attempt (see flowplugins.Permanent).
func inputError(err error) error {
	return flowplugins.Permanent(flowplugins.ErrorTypeInput, err)
}

// ErrSuspended signals to the orchestrator that this plugin step is parked and
// waiting for an external callback before the sub-workflow can advance.
var ErrSuspended = flowplugins.ErrSuspended
