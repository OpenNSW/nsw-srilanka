package plugins

import (
	"context"
	"log/slog"
	"net/url"

	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
)

// Package plugins hosts taskv2's dispatching plugins. Outbound calls are
// routed through remote.Manager so service base URLs, auth, and timeouts
// live in services.json rather than in template configs — template configs
// specify only service_id + path.
//
// In devMode, dispatch errors are logged and swallowed so local workflows
// can still progress when the receiving OGA portal isn't running.

// dispatchHelper bundles outbound HTTP behaviour shared by plugins in this
// package.
type dispatchHelper struct {
	manager        *remote.Manager
	backendBaseURL string
	devMode        bool
}

func newDispatchHelper(manager *remote.Manager, backendBaseURL string, devMode bool) *dispatchHelper {
	return &dispatchHelper{
		manager:        manager,
		backendBaseURL: backendBaseURL,
		devMode:        devMode,
	}
}

// callbackTasksURL is the URL the receiving OGA portal should call back into
// to advance the workflow once the officer has acted.
func (h *dispatchHelper) callbackTasksURL() string {
	joined, err := url.JoinPath(h.backendBaseURL, "/api/v1/tasks")
	if err != nil {
		slog.Error("taskv2 plugin: failed to build callback URL",
			"backendBaseURL", h.backendBaseURL, "error", err)
		return h.backendBaseURL + "/api/v1/tasks"
	}
	return joined
}

// post sends body as JSON to the resolved service+path. In devMode, dispatch
// errors are logged and swallowed.
func (h *dispatchHelper) post(ctx context.Context, serviceID, path string, body any) error {
	req := remote.Request{
		Method: "POST",
		Path:   path,
		Body:   body,
	}
	if err := h.manager.Call(ctx, serviceID, req, nil); err != nil {
		return h.dispatchOrSwallow(serviceID, path, err)
	}
	return nil
}

func (h *dispatchHelper) dispatchOrSwallow(serviceID, path string, err error) error {
	if h.devMode {
		slog.Warn("taskv2 plugin: dispatch failed (dev mode — swallowing)",
			"serviceId", serviceID, "path", path, "error", err)
		return nil
	}
	return err
}

type pluginContext = flowplugins.PluginContext

// ErrSuspended signals to the orchestrator that this plugin step is parked and
// waiting for an external callback before the sub-workflow can advance.
var ErrSuspended = flowplugins.ErrSuspended
