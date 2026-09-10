package asycuda

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every callback ASYCUDA sends is recorded with its body. It is the only record
// of what arrived: each handler logs its reading of a callback, not the callback
// itself, so without this a disagreement between the two cannot be settled.
func TestHandleWebhook_LogsEveryCallbackWithItsBody(t *testing.T) {
	for _, body := range []string{
		`{"eventType":"CUSDEC_INTEGRATED","payload":{"edgeId":"edge-1","integrated":false,` +
			`"errors":{"0":[{"code":7,"description":"Server error"}]}}}`,
		`{"eventType":"CDN_INTEGRATED","payload":{"edgeId":"edge-2","integrated":true,` +
			`"cdnRef":{"office":"CBEX1","year":"2026","serial":"C","number":28237}}}`,
		`{"eventType":"PAYMENT_CONFIRMED","payload":{"amountPaid":2035}}`,
		// An event nothing handles is exactly when the body matters most.
		`{"eventType":"SOMETHING_NEW","payload":{"whatever":true}}`,
	} {
		logs := &bytes.Buffer{}
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo})))

		handler := &Handler{}
		request := httptest.NewRequest(http.MethodPost, "/webhooks/slce", strings.NewReader(body))
		func() {
			// A handler with no services panics once it dispatches; the receipt
			// log happens first, which is the whole point of it being first.
			defer func() { _ = recover() }()
			handler.HandleWebhook(httptest.NewRecorder(), request)
		}()
		slog.SetDefault(previous)

		var found bool
		for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			if entry["msg"] != "slce: webhook received" {
				continue
			}
			found = true
			if entry["body"] != body {
				t.Errorf("logged body does not match what arrived\ngot:  %v\nwant: %s", entry["body"], body)
			}
		}
		if !found {
			t.Errorf("no receipt log for %s\ngot: %s", body, logs.String())
		}
	}
}
