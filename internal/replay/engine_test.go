package replay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInterpolate(t *testing.T) {
	r := New("", nil)
	r.Vars = map[string]any{"id": "abc", "n": 7}

	cases := map[string]string{
		"/x/{{id}}":         "/x/abc",
		"/x/{{id}}/y/{{n}}": "/x/abc/y/7",
		"/x/{{missing}}":    "/x/{{missing}}", // unknown vars are left intact
		"no placeholders":   "no placeholders",
	}
	for in, want := range cases {
		if got := r.interpolate(in); got != want {
			t.Errorf("interpolate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInterpolateValue(t *testing.T) {
	r := New("", nil)
	r.Vars = map[string]any{"cha": "adam-pvt-ltd"}

	in := map[string]any{
		"cha_company_id": "{{cha}}",
		"nested":         map[string]any{"k": "{{cha}}"},
		"list":           []any{"{{cha}}", "static"},
		"num":            42,
	}
	got := r.interpolateValue(in)
	want := map[string]any{
		"cha_company_id": "adam-pvt-ltd",
		"nested":         map[string]any{"k": "adam-pvt-ltd"},
		"list":           []any{"adam-pvt-ltd", "static"},
		"num":            42,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("interpolateValue = %#v, want %#v", got, want)
	}
}

func TestLoadFlow(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.json")
	mustWrite(t, good, `{"name":"f","steps":[{"name":"s","request":{"method":"GET","path":"/x"}}]}`)
	flow, err := LoadFlow(good)
	if err != nil {
		t.Fatalf("LoadFlow(good): %v", err)
	}
	if flow.Name != "f" || len(flow.Steps) != 1 {
		t.Fatalf("unexpected flow: %+v", flow)
	}

	empty := filepath.Join(dir, "empty.json")
	mustWrite(t, empty, `{"name":"f","steps":[]}`)
	if _, err := LoadFlow(empty); err == nil {
		t.Error("LoadFlow(empty): expected error for zero steps")
	}

	if _, err := LoadFlow(filepath.Join(dir, "nope.json")); err == nil {
		t.Error("LoadFlow(missing): expected error")
	}
}

func TestRunner_RequestExtractAndStatusAssertion(t *testing.T) {
	var gotActor, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = r.Header.Get(AuthActorHeader)
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "c-123"})
	}))
	defer srv.Close()

	r := New(srv.URL, srv.Client())
	flow := &Flow{Name: "t", Steps: []Step{
		{Name: "create", Request: &Request{
			Actor: "trader", Method: "POST", Path: "/api/v1/consignments",
			Body: map[string]any{"x": 1}, ExpectStatus: 201,
			Extract: map[string]string{"consignmentId": "id"},
		}},
	}}
	if err := r.Run(context.Background(), flow); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Vars["consignmentId"] != "c-123" {
		t.Errorf("extract: consignmentId = %v, want c-123", r.Vars["consignmentId"])
	}
	if gotActor != "trader" {
		t.Errorf("actor header = %q, want trader", gotActor)
	}
	if !strings.Contains(gotBody, `"x":1`) {
		t.Errorf("request body = %q, want it to contain x:1", gotBody)
	}

	// Status mismatch must fail the step.
	bad := &Flow{Name: "t", Steps: []Step{
		{Name: "create", Request: &Request{Method: "POST", Path: "/", ExpectStatus: 200}},
	}}
	if err := r.Run(context.Background(), bad); err == nil {
		t.Error("expected error on status mismatch (got 201, want 200)")
	}
}

func TestRunner_WaitMatchesNode(t *testing.T) {
	detail := `{"id":"c-1","state":"IN_PROGRESS","workflowNodes":[
		{"id":"task-init","state":"COMPLETED","workflowNodeTemplate":{"name":"[Trade] Initialize Consignment","type":"APPLICATION"}},
		{"id":"task-hs","state":"IN_PROGRESS","workflowNodeTemplate":{"name":"[Trade] Select HS Codes","type":"APPLICATION"}}
	]}`
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/consignments/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(detail))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := New(srv.URL, srv.Client())
	r.Vars[consignmentIDVar] = "c-1"

	if err := r.doWait(context.Background(), &Wait{Node: "Select HS Codes", State: "IN_PROGRESS", Into: "hsTask", Timeout: "2s"}); err != nil {
		t.Fatalf("doWait: %v", err)
	}
	if r.Vars["hsTask"] != "task-hs" {
		t.Errorf("wait Into: hsTask = %v, want task-hs", r.Vars["hsTask"])
	}

	// A node that never matches must time out.
	err := r.doWait(context.Background(), &Wait{Node: "Nonexistent", State: "IN_PROGRESS", Timeout: "1ms"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got %v", err)
	}
}

func TestLookupPath(t *testing.T) {
	obj := map[string]any{
		"id":          "top",
		"consignment": map[string]any{"id": "nested", "deep": map[string]any{"k": "v"}},
	}
	cases := []struct {
		path    string
		want    any
		present bool
	}{
		{"id", "top", true},
		{"consignment.id", "nested", true},
		{"consignment.deep.k", "v", true},
		{"missing", nil, false},
		{"consignment.missing", nil, false},
		{"id.x", nil, false}, // descending into a non-object
	}
	for _, c := range cases {
		got, ok := lookupPath(obj, c.path)
		if ok != c.present || (ok && got != c.want) {
			t.Errorf("lookupPath(%q) = (%v, %v), want (%v, %v)", c.path, got, ok, c.want, c.present)
		}
	}
}

func TestDoCallback(t *testing.T) {
	r := New("", nil)
	// Without an Agency the callback step is an error.
	if err := r.doCallback(context.Background(), &Callback{TaskVar: "myTask"}); err == nil {
		t.Error("expected error when Agency is nil")
	}

	fake := &fakeAgency{}
	r.Agency = fake
	// Missing taskVar is an error.
	if err := r.doCallback(context.Background(), &Callback{TaskVar: "myTask", Command: "approve"}); err == nil {
		t.Error("expected error when taskVar variable is not set")
	}

	r.Vars["myTask"] = "task-abc"
	r.Vars["ref"] = "REF-9"
	cb := &Callback{
		TaskVar: "myTask",
		Command: "approve",
		Content: map[string]any{"application_review_outcome": "approve", "reference_number": "{{ref}}"},
		Timeout: "5s",
	}
	if err := r.doCallback(context.Background(), cb); err != nil {
		t.Fatalf("doCallback: %v", err)
	}
	if fake.taskID != "task-abc" {
		t.Errorf("agency taskID = %q", fake.taskID)
	}
	if fake.command != "approve" {
		t.Errorf("agency command = %q", fake.command)
	}
	if fake.content["reference_number"] != "REF-9" {
		t.Errorf("callback content not interpolated: %v", fake.content)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

type fakeAgency struct {
	taskID  string
	command string
	content map[string]any
}

func (f *fakeAgency) Respond(_ context.Context, taskID, command string, content map[string]any, _ time.Duration) error {
	f.taskID = taskID
	f.command = command
	f.content = content
	return nil
}

func TestDoPay(t *testing.T) {
	r := New("", nil)
	// Without a PaymentGateway the pay step is an error.
	if err := r.doPay(context.Background(), &Pay{TaskVar: "payTask"}); err == nil {
		t.Error("expected error when Gateway is nil")
	}

	fake := &fakeGateway{}
	r.PaymentGateway = fake
	// Missing taskVar is an error.
	if err := r.doPay(context.Background(), &Pay{TaskVar: "payTask"}); err == nil {
		t.Error("expected error when taskVar is not set")
	}

	r.Vars["payTask"] = "fcau_2_0_pay_fee:abc"
	if err := r.doPay(context.Background(), &Pay{TaskVar: "payTask"}); err != nil {
		t.Fatalf("doPay: %v", err)
	}
	if fake.taskID != "fcau_2_0_pay_fee:abc" {
		t.Errorf("gateway taskID = %q", fake.taskID)
	}
	if fake.status != "paid" { // default when Pay.Status is empty
		t.Errorf("gateway status = %q, want default paid", fake.status)
	}
}

type fakeGateway struct {
	taskID string
	status string
}

func (f *fakeGateway) Pay(_ context.Context, taskID, status string, _ time.Duration) error {
	f.taskID = taskID
	f.status = status
	return nil
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
