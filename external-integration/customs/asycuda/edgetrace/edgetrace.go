// Package edgetrace records the exchange with SLC Edge in full: what was sent,
// what came back, and the callbacks they push afterwards.
//
// Everything here is DEBUG, and deliberately so. These payloads are the
// trader's declaration — names, addresses, invoice values, container numbers —
// which is not something to accumulate in the log of a running deployment. It
// is also the only thing that answers the first question asked of any
// integration defect: what exactly did we send them, and what did they say.
//
// So it is a switch rather than a default. Turn it on for as long as the
// question takes to answer:
//
//	SERVER_LOG_LEVEL=debug
//
// Each function checks the level before doing any work, so nothing is
// marshalled while the switch is off.
package edgetrace

import (
	"context"
	"encoding/json"
	"log/slog"
)

// Request records the payload on its way to SLC Edge.
//
// body is the value that will be encoded rather than the encoded bytes: the
// interpreters build it and hand it to the plugin, which does the encoding, so
// this is the last point the integration itself can see it.
func Request(ctx context.Context, endpoint string, body any) {
	if !enabled(ctx) {
		return
	}

	slog.DebugContext(ctx, "slce: request", "endpoint", endpoint, "body", render(body))
}

// Response records what SLC Edge answered, parsed, along with any transport
// error. A failed call is traced too — an empty body beside a timeout is itself
// the answer.
func Response(ctx context.Context, endpoint string, resp map[string]any, callErr error) {
	if !enabled(ctx) {
		return
	}

	slog.DebugContext(ctx, "slce: response", "endpoint", endpoint, "error", callErr, "body", render(resp))
}

// Webhook records a callback as it arrived, before anything decodes it, so a
// payload that fails to parse is still readable in full.
func Webhook(ctx context.Context, path string, body []byte) {
	if !enabled(ctx) {
		return
	}

	slog.DebugContext(ctx, "slce: webhook received", "path", path, "bytes", len(body), "body", string(body))
}

func enabled(ctx context.Context) bool {
	return slog.Default().Enabled(ctx, slog.LevelDebug)
}

// render marshals a payload for the log. A value that cannot be marshalled is
// described rather than dropped: this runs on the path where something is
// already being investigated, and a silent gap is worse than a line saying why.
func render(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "<unrenderable: " + err.Error() + ">"
	}
	return string(encoded)
}
