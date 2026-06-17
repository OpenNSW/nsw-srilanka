// Package replay is a small, data-driven engine for end-to-end tests: it
// executes an ordered list of steps (defined in a JSON flow file) against a
// running NSW backend. Each flow is data, so new scenarios (FCAU, NPQS, …) are
// authored as config rather than code.
//
// The engine speaks only HTTP + JSON, so it is independent of the backend's
// internals and could later drive a black-box target. Per-step identity is
// selected by the "actor" field, surfaced to the server via the
// X-Auth-Actor header (the in-process test wires a stub-auth middleware that
// reads it; a black-box transport would map actors to real tokens instead).
//
// Step kinds:
//   - request:  issue an HTTP call, assert the status, optionally extract
//     response fields into variables for later steps.
//   - wait:     poll the consignment detail until a workflow node (matched by
//     display-name substring) reaches a state; optionally capture its task id.
//   - callback: drive a mock external agency to post its OGA callback for a
//     parked task, advancing an EXTERNAL_REVIEW step.
//
// Strings in paths and request bodies may reference variables as {{name}}.
package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// AuthActorHeader carries the replay actor key to the stub-auth middleware.
const AuthActorHeader = "X-Auth-Actor"

// consignmentIDVar is the well-known variable holding the consignment id that
// wait/callback steps poll against. A create step typically extracts it.
const consignmentIDVar = "consignmentId"

// Flow is an ordered list of steps loaded from a JSON file.
type Flow struct {
	Name  string `json:"name"`
	Steps []Step `json:"steps"`
}

// Step is exactly one of request, wait, or callback.
type Step struct {
	Name     string    `json:"name"`
	Request  *Request  `json:"request,omitempty"`
	Wait     *Wait     `json:"wait,omitempty"`
	Callback *Callback `json:"callback,omitempty"`
}

// Request issues an HTTP call as a given actor.
type Request struct {
	Actor        string            `json:"actor"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Body         any               `json:"body,omitempty"`
	ExpectStatus int               `json:"expectStatus,omitempty"` // default 200
	Extract      map[string]string `json:"extract,omitempty"`      // var -> response field path (dot-notation, e.g. "consignment.id")
}

// Wait polls the consignment detail until a workflow node matches.
type Wait struct {
	Node    string `json:"node"`           // substring match on the node display name
	State   string `json:"state"`          // required node state (empty = any)
	Into    string `json:"into,omitempty"` // store the matched node's task id here
	Timeout string `json:"timeout,omitempty"`
}

// Callback drives the mock agency to respond to a parked EXTERNAL_REVIEW task.
type Callback struct {
	TaskCode string         `json:"taskCode"` // substring match on the inject's taskCode
	Content  map[string]any `json:"content"`  // reviewer payload sent back to NSW
	Timeout  string         `json:"timeout,omitempty"`
}

// Agency is the mock external agency the callback step drives. The in-process
// test provides an implementation backed by a stub HTTP server.
type Agency interface {
	// Respond waits (up to timeout) for an inject whose taskCode contains
	// taskCodeContains, then posts the OGA callback envelope back into NSW with
	// content as the reviewer payload.
	Respond(ctx context.Context, taskCodeContains string, content map[string]any, timeout time.Duration) error
}

// LoadFlow reads and parses a JSON flow file.
func LoadFlow(path string) (*Flow, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read flow %s: %w", path, err)
	}
	var f Flow
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse flow %s: %w", path, err)
	}
	if len(f.Steps) == 0 {
		return nil, fmt.Errorf("flow %s has no steps", path)
	}
	return &f, nil
}

// Runner executes flows against a backend reachable at BaseURL.
type Runner struct {
	BaseURL string
	Client  *http.Client
	Vars    map[string]any
	Logf    func(string, ...any)
	Agency  Agency
}

// New builds a Runner with an empty variable store and a no-op logger.
func New(baseURL string, client *http.Client) *Runner {
	if client == nil {
		client = http.DefaultClient
	}
	return &Runner{
		BaseURL: baseURL,
		Client:  client,
		Vars:    map[string]any{},
		Logf:    func(string, ...any) {},
	}
}

// Run executes every step in order, returning the first error encountered.
func (r *Runner) Run(ctx context.Context, flow *Flow) error {
	for i, step := range flow.Steps {
		label := fmt.Sprintf("step %d/%d %q", i+1, len(flow.Steps), step.Name)
		var err error
		switch {
		case step.Request != nil:
			err = r.doRequest(ctx, step.Request)
		case step.Wait != nil:
			err = r.doWait(ctx, step.Wait)
		case step.Callback != nil:
			err = r.doCallback(ctx, step.Callback)
		default:
			err = fmt.Errorf("step has no request/wait/callback")
		}
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		r.Logf("✓ %s", label)
	}
	return nil
}

func (r *Runner) doRequest(ctx context.Context, req *Request) error {
	path := r.interpolate(req.Path)

	var rdr io.Reader
	if req.Body != nil {
		raw, err := json.Marshal(r.interpolateValue(req.Body))
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, r.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.Actor != "" {
		httpReq.Header.Set(AuthActorHeader, req.Actor)
	}

	resp, err := r.Client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	want := req.ExpectStatus
	if want == 0 {
		want = http.StatusOK
	}
	if resp.StatusCode != want {
		return fmt.Errorf("%s %s: got status %d, want %d: %s", req.Method, path, resp.StatusCode, want, truncate(respBody))
	}

	if len(req.Extract) > 0 {
		var obj map[string]any
		if err := json.Unmarshal(respBody, &obj); err != nil {
			return fmt.Errorf("extract: response must be a JSON object: %w (body=%s)", err, truncate(respBody))
		}
		for varName, path := range req.Extract {
			v, ok := lookupPath(obj, path)
			if !ok {
				return fmt.Errorf("extract: path %q not present in response", path)
			}
			r.Vars[varName] = v
		}
	}
	return nil
}

func (r *Runner) doWait(ctx context.Context, w *Wait) error {
	timeout, err := parseTimeout(w.Timeout, 45*time.Second)
	if err != nil {
		return err
	}
	cid, _ := r.Vars[consignmentIDVar].(string)
	if cid == "" {
		return fmt.Errorf("wait: %q variable not set (extract it in an earlier step)", consignmentIDVar)
	}

	deadline := time.Now().Add(timeout)
	var last consignmentDetail
	for {
		d, err := r.fetchDetail(ctx, cid)
		if err != nil {
			return err
		}
		last = d
		for _, n := range d.WorkflowNodes {
			if strings.Contains(n.Template.Name, w.Node) && (w.State == "" || n.State == w.State) {
				if w.Into != "" {
					r.Vars[w.Into] = n.ID
				}
				return nil
			}
		}
		if time.Now().After(deadline) {
			buf, _ := json.MarshalIndent(last.WorkflowNodes, "", "  ")
			return fmt.Errorf("timed out after %s waiting for node %q state %q; current nodes:\n%s", timeout, w.Node, w.State, buf)
		}
		if err := sleep(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
}

func (r *Runner) doCallback(ctx context.Context, cb *Callback) error {
	if r.Agency == nil {
		return fmt.Errorf("callback step requires an Agency responder (none configured)")
	}
	timeout, err := parseTimeout(cb.Timeout, 30*time.Second)
	if err != nil {
		return err
	}
	return r.Agency.Respond(ctx, cb.TaskCode, r.interpolateMap(cb.Content), timeout)
}

// ── consignment detail polling ──────────────────────────────────────────────

type nodeView struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Template struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"workflowNodeTemplate"`
}

type consignmentDetail struct {
	ID            string     `json:"id"`
	State         string     `json:"state"`
	ChaCompanyID  string     `json:"chaCompanyId"`
	WorkflowNodes []nodeView `json:"workflowNodes"`
}

func (r *Runner) fetchDetail(ctx context.Context, consignmentID string) (consignmentDetail, error) {
	var d consignmentDetail
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+"/api/v1/consignments/"+consignmentID, nil)
	if err != nil {
		return d, err
	}
	req.Header.Set(AuthActorHeader, "trader")
	resp, err := r.Client.Do(req)
	if err != nil {
		return d, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return d, fmt.Errorf("read consignment detail response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return d, fmt.Errorf("GET consignment %s: status %d: %s", consignmentID, resp.StatusCode, truncate(body))
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return d, fmt.Errorf("decode consignment detail: %w", err)
	}
	return d, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

var varRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// interpolate replaces {{name}} tokens in s with the string form of Vars[name].
func (r *Runner) interpolate(s string) string {
	return varRe.ReplaceAllStringFunc(s, func(m string) string {
		name := varRe.FindStringSubmatch(m)[1]
		if v, ok := r.Vars[name]; ok {
			return fmt.Sprint(v)
		}
		return m
	})
}

// interpolateValue walks maps/slices/strings, interpolating string values.
func (r *Runner) interpolateValue(v any) any {
	switch t := v.(type) {
	case string:
		return r.interpolate(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = r.interpolateValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = r.interpolateValue(val)
		}
		return out
	default:
		return v
	}
}

func (r *Runner) interpolateMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return r.interpolateValue(m).(map[string]any)
}

func parseTimeout(s string, def time.Duration) (time.Duration, error) {
	if s == "" {
		return def, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: %w", s, err)
	}
	return d, nil
}

func sleep(ctx context.Context, d time.Duration) error {
	// Use a managed timer (not time.After) so it is released immediately when
	// ctx is cancelled, rather than lingering until d elapses — avoids timer
	// accumulation across many poll iterations.
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// lookupPath resolves a dot-notation path (e.g. "consignment.id") against a
// decoded JSON object, descending through nested objects. Returns false if any
// segment is missing or a non-object is encountered mid-path.
func lookupPath(obj map[string]any, path string) (any, bool) {
	var cur any = obj
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func truncate(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
