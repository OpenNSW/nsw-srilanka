package replay_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// injectRequest mirrors the payload the EXTERNAL_REVIEW plugin POSTs to an
// external agency: the parked task's id, the task code, the consignment id,
// the mapped submission data, and the callback URL.
type injectRequest struct {
	TaskID        string         `json:"taskId"`
	TaskCode      string         `json:"taskCode"`
	ConsignmentID string         `json:"consignmentId"`
	Data          map[string]any `json:"data"`
	ServiceURL    string         `json:"serviceUrl"`
}

const agencyPollInterval = 300 * time.Millisecond

// mockAgency is a generic controllable stand-in for any external OGA agency.
// It receives the system's inject and, when a replay `callback` step fires,
// posts {command, payload} back to the NSW task endpoint to complete the
// parked EXTERNAL_REVIEW step. It implements replay.Agency.
//
// The callback is authenticated with a real agency bearer token (the app runs
// the production authn middleware). callbackBase and bearer are set by the
// harness after the app server starts.
type mockAgency struct {
	server *httptest.Server
	client *http.Client

	mu      sync.Mutex
	injects map[string]injectRequest // keyed by taskId

	// Set by the harness after the app server starts.
	callbackBase string // the in-process NSW app base URL
	bearer       string // agency SERVICE token for the callback Authorization header
	logf         func(string, ...any)
}

// newMockAgency starts the agency HTTP server (so it is reachable before the
// app's first inject). The harness sets callbackBase/bearer after the app starts.
func newMockAgency(t *testing.T) *mockAgency {
	t.Helper()
	a := &mockAgency{
		client:  &http.Client{Timeout: 10 * time.Second},
		injects: make(map[string]injectRequest),
		logf:    t.Logf,
	}
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
	a.injects[req.TaskID] = req
	a.mu.Unlock()
	a.logf("mock-agency: received inject taskCode=%s taskId=%s consignmentId=%s", req.TaskCode, req.TaskID, req.ConsignmentID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "taskId": req.TaskID})
}

func (a *mockAgency) findInject(taskID string) (injectRequest, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	inj, ok := a.injects[taskID]
	return inj, ok
}

// Respond implements replay.Agency: wait (up to timeout) for the inject for
// taskID, then post {command, payload:content} to the NSW task endpoint to
// complete the parked EXTERNAL_REVIEW step.
func (a *mockAgency) Respond(ctx context.Context, taskID, command string, content map[string]any, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(agencyPollInterval)
	defer ticker.Stop()

	var inj injectRequest
	for {
		var ok bool
		if inj, ok = a.findInject(taskID); ok {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("mock-agency: no inject for taskId %q within %s", taskID, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

	body, err := json.Marshal(map[string]any{"command": command, "payload": content})
	if err != nil {
		return fmt.Errorf("mock-agency: marshal callback: %w", err)
	}

	url := a.callbackBase + "/api/v1/tasks/" + inj.TaskID
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
		return fmt.Errorf("mock-agency: callback to %s got status %d: %s", url, resp.StatusCode, string(rb))
	}
	a.logf("mock-agency: callback delivered for task %s command=%s (status %d)", inj.TaskID, command, resp.StatusCode)
	return nil
}
