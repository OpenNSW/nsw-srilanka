package asycuda

// TEMPORARY DEBUGGING CODE: REMOVE IT ONCE THE DEBUGGING IS DONE.
//
// Sends a copy of every SLC Edge callback this server receives on
// POST /webhooks/slce to the same endpoint of another deployment, signed in as
// that deployment's client. It runs only when config.yaml lists a service with
// the id "slce-debug-forward":
//
//	services:
//	  - id: slce-debug-forward
//	    url: https://other-deployment.example.com
//	    timeout: 30s
//	    auth:
//	      type: oauth2
//	      oauth2:
//	        tokenURL: https://other-idp.example.com/oauth2/token
//	        clientID: OTHER_CLIENT_ID
//	        clientSecret: "{{env:SLCE_DEBUG_FORWARD_SECRET}}"
//	        scopes: [nsw:slce-webhooks:write]
//
// The copy is sent in the background: it never changes the answer SLC Edge gets,
// and a failed copy is only logged.
//
// To remove it:
//  1. delete this file;
//  2. in handler.go, delete the DebugForward field and the forwardDebugCopy call;
//  3. in internal/bootstrap/app.go, delete the block marked TEMPORARY that sets
//     DebugForward;
//  4. delete the slce-debug-forward service from config.yaml.

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/remote"
)

// DebugForwardServiceID is the config.yaml service the copies are sent to.
const DebugForwardServiceID = "slce-debug-forward"

// forwardDebugCopy sends body to the slce-debug-forward service, if one is set.
func (h *Handler) forwardDebugCopy(ctx context.Context, body []byte) {
	if h.DebugForward == nil {
		return
	}
	// Not cancelled when SLC Edge's request ends; the service's timeout bounds it.
	ctx = context.WithoutCancel(ctx)
	go func() {
		resp, err := h.DebugForward.CallRaw(ctx, DebugForwardServiceID, remote.Request{
			Method: http.MethodPost,
			Path:   "/webhooks/slce",
			Body:   remote.RawBody{Data: body, ContentType: "application/json"},
		})
		if err != nil {
			slog.WarnContext(ctx, "slce debug forward: copy not sent", "error", err)
			return
		}
		slog.InfoContext(ctx, "slce debug forward: copy sent", "status", resp.StatusCode, "response", string(resp.Body))
	}()
}
