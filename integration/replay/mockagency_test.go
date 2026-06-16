package replay_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// injectRequest mirrors the payload the EXTERNAL_REVIEW plugin POSTs to an
// external agency (nsw-agency's InjectRequest): the parked task's id, the task
// code, the consignment id, the mapped submission data, and the callback URL.
type injectRequest struct {
	TaskID        string         `json:"taskId"`
	TaskCode      string         `json:"taskCode"`
	ConsignmentID string         `json:"consignmentId"`
	Data          map[string]any `json:"data"`
	ServiceURL    string         `json:"serviceUrl"`
}

// mockAgency is a controllable stand-in for an external OGA agency (e.g. FCAU).
// It receives the system's `inject` and, when a replay `callback` step fires,
// posts the OGA callback envelope back into NSW. It implements replay.Agency.
//
// The callback is authenticated with a REAL `fcau` bearer token (the app runs
// the production authn middleware), and is sent to the harness-provided
// callbackBase — NOT the inject's serviceUrl, which points at cfg.Server.ServiceURL
// rather than the in-process httptest app.
type mockAgency struct {
	server *httptest.Server
	client *http.Client

	mu      sync.Mutex
	injects []injectRequest

	// Set by the harness after the app server starts.
	callbackBase string // the in-process NSW app base URL
	bearer       string // real `fcau` token for the callback Authorization header
	logf         func(string, ...any)
}

// newMockAgency starts the agency HTTP server (so it is reachable before the
// app's first inject). The harness sets callbackBase/bearer after the app starts.
func newMockAgency(t *testing.T) *mockAgency {
	t.Helper()
	a := &mockAgency{client: &http.Client{Timeout: 10 * time.Second}, logf: t.Logf}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/inject", a.handleInject)
	a.server = httptest.NewServer(mux)
	t.Cleanup(a.server.Close)
	return a
}

func (a *mockAgency) handleInject(w http.ResponseWriter, r *http.Request) {
	var req injectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad inject: "+err.Error(), http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	a.injects = append(a.injects, req)
	a.mu.Unlock()
	a.logf("mock-agency: received inject taskCode=%s taskId=%s consignmentId=%s", req.TaskCode, req.TaskID, req.ConsignmentID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "taskId": req.TaskID})
}

func (a *mockAgency) findInject(taskCodeContains string) (injectRequest, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := len(a.injects) - 1; i >= 0; i-- { // most-recent first
		if strings.Contains(a.injects[i].TaskCode, taskCodeContains) {
			return a.injects[i], true
		}
	}
	return injectRequest{}, false
}

// Respond implements replay.Agency: wait (up to timeout) for an inject whose
// taskCode matches, then post the OGA callback envelope back into NSW with
// content as the reviewer payload.
func (a *mockAgency) Respond(ctx context.Context, taskCodeContains string, content map[string]any, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var inj injectRequest
	for {
		var ok bool
		if inj, ok = a.findInject(taskCodeContains); ok {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("mock-agency: no inject with taskCode containing %q within %s", taskCodeContains, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}

	envelope := map[string]any{
		"task_id":        inj.TaskID,
		"consignment_id": inj.ConsignmentID,
		"payload": map[string]any{
			"action":  "AGENCY_VERIFICATION",
			"content": content,
		},
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("mock-agency: marshal callback: %w", err)
	}

	// Post to the legacy OGA route; the handler reads task_id from the body and
	// unwraps payload.content (unwrapOGACallback).
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.callbackBase+"/api/v1/tasks", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+a.bearer)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("mock-agency: callback POST: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mock-agency: callback to %s got status %d: %s", a.callbackBase+"/api/v1/tasks", resp.StatusCode, string(rb))
	}
	a.logf("mock-agency: callback delivered for task %s (status %d)", inj.TaskID, resp.StatusCode)
	return nil
}
