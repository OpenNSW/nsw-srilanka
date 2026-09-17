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

// dispatchHelper bundles outbound HTTP behaviour shared by plugins in this
// package.
type dispatchHelper struct {
	manager        *remote.Manager
	backendBaseURL string
}

func newDispatchHelper(manager *remote.Manager, backendBaseURL string) *dispatchHelper {
	return &dispatchHelper{
		manager:        manager,
		backendBaseURL: backendBaseURL,
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

// post sends body as JSON to the resolved service+path. Errors always
// propagate: this dispatch is a precondition for ErrSuspended (the caller is
// about to park the subtask awaiting a callback that only the receiving
// service can send), so a failed dispatch must fail the step rather than
// suspend it — the task engine's own retry is what lets the workflow recover
// once the receiving service comes up.
func (h *dispatchHelper) post(ctx context.Context, serviceID, path string, body any) error {
	req := remote.Request{
		Method: "POST",
		Path:   path,
		Body:   remote.JSONBody{V: body},
	}
	return h.manager.Call(ctx, serviceID, req, nil)
}

type pluginContext = flowplugins.PluginContext

// ErrSuspended signals to the orchestrator that this plugin step is parked and
// waiting for an external callback before the sub-workflow can advance.
var ErrSuspended = flowplugins.ErrSuspended
