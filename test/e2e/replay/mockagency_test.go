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

const agencyPollInterval = 300 * time.Millisecond

// storedInject holds the raw inject payload received from the NSW app, tagged
// with which agency received it so Respond can route the callback correctly.
type storedInject struct {
	agencyID string
	taskID   string
	body     map[string]any
}

// mockAgency is a generic controllable stand-in for any external OGA agency.
// It is driven by []AgencyConfig: each agency registers its own inject endpoint
// and callback wire format. When a replay `callback` step fires, Respond posts
// the configured payload back to the NSW task endpoint. It implements replay.Agency.
//
// callbackBase and bearers are set by the harness after the app server starts.
type mockAgency struct {
	server *httptest.Server
	client *http.Client

	mu      sync.Mutex
	injects map[string]storedInject // keyed by taskId

	configs map[string]AgencyConfig // agencyID -> config

	// Set by the harness after the app server starts.
	callbackBase string            // the in-process NSW app base URL
	bearers      map[string]string // agencyID -> SERVICE bearer token
	logf         func(string, ...any)
}

// newMockAgency starts the agency HTTP server (reachable before the app's first
// inject) and registers one inject endpoint per agency config. The harness sets
// callbackBase and bearers after the app starts.
func newMockAgency(t *testing.T, configs []AgencyConfig) *mockAgency {
	t.Helper()
	a := &mockAgency{
		client:  &http.Client{Timeout: 10 * time.Second},
		injects: make(map[string]storedInject),
		bearers: make(map[string]string),
		configs: make(map[string]AgencyConfig, len(configs)),
		logf:    t.Logf,
	}
	for _, cfg := range configs {
		a.configs[cfg.ID] = cfg
	}

	mux := http.NewServeMux()
	for _, cfg := range configs {
		cfg := cfg // capture
		// Prefix the path with the agency ID so all agencies can share the same
		// real-world inject path (e.g. /api/v1/inject) on one mock server.
		// writeServicesConfig points each agency at agencyURL/<id>, so the NSW
		// app sends to /<id>/api/v1/inject — matching the pattern below.
		parts := strings.SplitN(cfg.Inbound.Endpoint, " ", 2)
		mux.HandleFunc(parts[0]+" /"+cfg.ID+parts[1], func(w http.ResponseWriter, r *http.Request) {
			a.handleInject(w, r, cfg.ID, cfg.Inbound.TaskIDField)
		})
	}
	a.server = httptest.NewServer(mux)
	t.Cleanup(a.server.Close)
	return a
}

func (a *mockAgency) handleInject(w http.ResponseWriter, r *http.Request, agencyID, taskIDField string) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad inject: "+err.Error(), http.StatusBadRequest)
		return
	}
	taskID, _ := body[taskIDField].(string)
	if taskID == "" {
		http.Error(w, "missing "+taskIDField, http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	a.injects[taskID] = storedInject{agencyID: agencyID, taskID: taskID, body: body}
	a.mu.Unlock()
	a.logf("mock-agency[%s]: received inject taskId=%s", agencyID, taskID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "taskId": taskID})
}

func (a *mockAgency) findInject(taskID string) (storedInject, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	inj, ok := a.injects[taskID]
	return inj, ok
}

// Respond implements replay.Agency: wait (up to timeout) for the inject for
// taskID, then post the configured callback payload to the NSW task endpoint.
func (a *mockAgency) Respond(ctx context.Context, taskID, command string, content map[string]any, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(agencyPollInterval)
	defer ticker.Stop()

	var inj storedInject
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

	cfg, ok := a.configs[inj.agencyID]
	if !ok {
		return fmt.Errorf("mock-agency: no config found for agency ID %q", inj.agencyID)
	}
	callbackURL := a.callbackBase + strings.Replace(cfg.Outbound.CallbackPath, "{taskId}", taskID, 1)
	body, err := json.Marshal(map[string]any{
		cfg.Outbound.CommandField: command,
		cfg.Outbound.PayloadField: content,
	})
	if err != nil {
		return fmt.Errorf("mock-agency: marshal callback: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer := a.bearers[inj.agencyID]; bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("mock-agency: callback POST: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mock-agency: callback to %s got status %d: %s", callbackURL, resp.StatusCode, string(rb))
	}
	a.logf("mock-agency[%s]: callback delivered for task %s command=%s (status %d)", inj.agencyID, taskID, command, resp.StatusCode)
	return nil
}
