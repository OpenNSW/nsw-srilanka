package caseflow

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// clientFlows lists notice for tnsw, owned by officer, in CDA's OU.
var clientFlows = ClientFlows{Enabled: true, OUHandle: "cda", Workflows: []ClientFlow{{ID: "notice", Clients: []string{"tnsw"}, Owner: "officer"}}}

// catalogClients is the catalog's Clients map: tnsw calls as NSW_TO_CDA.
var catalogClients = map[string]string{"tnsw": "NSW_TO_CDA", "fcau": "FCAU_TO_NSW"}

func newClientHandler(t *testing.T, list ClientFlows) (*ClientHandler, *memRepo, *startCapture) {
	t.Helper()
	repo, wm := newMemRepo(), &startCapture{}
	return NewClientHandler(newService(t, repo, wm), list, catalogClients), repo, wm
}

// post sends body to the route for case C1, as p (unauthenticated when nil).
func post(h *ClientHandler, p *authn.Principal, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/internal/cases/C1/flows", strings.NewReader(body))
	r.SetPathValue("caseId", "C1")
	if p != nil {
		r = r.WithContext(authn.ContextWithPrincipal(r.Context(), p))
	}
	rec := httptest.NewRecorder()
	h.HandleStart(rec, r)
	return rec
}

var tnsw = &authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA"}

const startBody = `{"workflowId":"notice","flowId":"task-1","input":{"hs":"0801"},"callbackToken":"tok-1"}`

func TestClientHandlerStartsTheFlow(t *testing.T) {
	h, repo, wm := newClientHandler(t, clientFlows)
	rec := post(h, tnsw, startBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var f Flow
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil || f.WorkflowID != "task-1" || f.CaseID != "C1" || f.TemplateID != "notice" {
		t.Fatalf("flow = %+v, err = %v", f, err)
	}
	if _, ok := repo.flows["task-1"]; !ok {
		t.Error("flow task-1 was not recorded")
	}
	if len(wm.starts) != 1 || wm.starts[0].id != "task-1" {
		t.Fatalf("starts = %+v", wm.starts)
	}
	// The agencies' workflows map these names.
	vars := wm.starts[0].vars
	for name, want := range map[string]any{"taskId": "task-1", "taskCode": "notice", "consignmentId": "C1", "callbackToken": "tok-1"} {
		if vars[name] != want {
			t.Errorf("vars[%q] = %v, want %v", name, vars[name], want)
		}
	}
	if n, ok := vars["notification"].(map[string]any); !ok || n["hs"] != "0801" {
		t.Errorf("notification = %v, want the input", vars["notification"])
	}
	if len(repo.owners) != 1 || repo.owners[0] != "C1/officer/cda" {
		t.Errorf("owners = %v, want C1 owned by cda as officer", repo.owners)
	}
}

// The request's attributes are stored with the flow, and caseColumns fills the case's
// name from them.
func TestClientHandlerRecordsTheCaseAttributes(t *testing.T) {
	list := clientFlows
	list.CaseColumns = map[string]string{CaseColumnName: "/traderCompany/name"}
	// cases.name holds 255 characters, which a multibyte name passes at twice the bytes.
	atLimit, tooLong := strings.Repeat("é", maxCaseNameLength), strings.Repeat("a", maxCaseNameLength+1)
	for name, tc := range map[string]struct {
		attributes string
		wantName   string
	}{
		"name mapped":     {`{"traderCompany":{"name":" Adam PVT LTD "}}`, "Adam PVT LTD"},
		"no such value":   {`{"flow":"EXPORT"}`, ""},
		"not a string":    {`{"traderCompany":{"name":7}}`, ""},
		"not an object":   {`{"traderCompany":"Adam PVT LTD"}`, ""},
		"blank":           {`{"traderCompany":{"name":"  "}}`, ""},
		"unmapped stored": {`{"traderCompany":{"name":"Adam PVT LTD"},"destination":"MY"}`, "Adam PVT LTD"},
		"at the limit":    {`{"traderCompany":{"name":"` + atLimit + `"}}`, atLimit},
		"too long":        {`{"traderCompany":{"name":"` + tooLong + `"}}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			h, repo, _ := newClientHandler(t, list)
			body := `{"workflowId":"notice","flowId":"task-1","attributes":` + tc.attributes + `}`
			if rec := post(h, tnsw, body); rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if len(repo.cases) != 1 {
				t.Fatalf("case info = %v, want one", repo.cases)
			}
			var want map[string]any
			if err := json.Unmarshal([]byte(tc.attributes), &want); err != nil {
				t.Fatal(err)
			}
			if got := repo.cases[0]; !reflect.DeepEqual(got.Attributes, want) || got.Name != tc.wantName {
				t.Errorf("case info = %+v, want attributes %v and name %q", got, want, tc.wantName)
			}
		})
	}
}

// Without caseColumns the attributes are stored and no column is filled; without
// attributes nothing is written to the case.
func TestClientHandlerCaseAttributesUnmapped(t *testing.T) {
	h, repo, _ := newClientHandler(t, clientFlows)
	if rec := post(h, tnsw, `{"workflowId":"notice","flowId":"task-1","attributes":{"traderCompany":{"name":"Adam PVT LTD"}}}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(repo.cases) != 1 || repo.cases[0].Name != "" || repo.cases[0].Attributes == nil {
		t.Errorf("case info = %v, want the attributes and no name", repo.cases)
	}

	h, repo, _ = newClientHandler(t, clientFlows)
	if rec := post(h, tnsw, startBody); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(repo.cases) != 0 {
		t.Errorf("case info = %v, want none", repo.cases)
	}
}

// No callback token, no variable: a workflow that calls back by it fails at the start.
func TestClientHandlerWithoutCallbackToken(t *testing.T) {
	h, _, wm := newClientHandler(t, clientFlows)
	if rec := post(h, tnsw, `{"workflowId":"notice","flowId":"task-1"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if _, ok := wm.starts[0].vars["callbackToken"]; ok {
		t.Error("callbackToken seeded without one in the request")
	}
}

// A repeat of the same request is answered from the recorded flow, which has its owner.
func TestClientHandlerRepeat(t *testing.T) {
	h, repo, wm := newClientHandler(t, clientFlows)
	for range 2 {
		if rec := post(h, tnsw, startBody); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	}
	if len(wm.starts) != 1 {
		t.Errorf("starts = %d, want 1", len(wm.starts))
	}
	if len(repo.owners) != 1 {
		t.Errorf("owners = %v, want the one recorded with the flow", repo.owners)
	}
}

// A flow the caller may not start is 404, as one that does not exist, and starts nothing.
func TestClientHandlerRefuses(t *testing.T) {
	off := clientFlows
	off.Enabled = false
	for name, tc := range map[string]struct {
		list ClientFlows
		p    *authn.Principal
		body string
	}{
		"list off":        {off, tnsw, startBody},
		"not listed":      {clientFlows, tnsw, `{"workflowId":"export","flowId":"task-1"}`},
		"another client":  {clientFlows, &authn.Principal{Kind: authn.KindClient, ClientID: "FCAU_TO_NSW"}, startBody},
		"unknown client":  {clientFlows, &authn.Principal{Kind: authn.KindClient, ClientID: "SOMEONE"}, startBody},
		"a user":          {clientFlows, &authn.Principal{Kind: authn.KindUser, OUHandle: "cda", Roles: []string{"Officer"}}, startBody},
		"unauthenticated": {clientFlows, nil, startBody},
	} {
		t.Run(name, func(t *testing.T) {
			h, repo, wm := newClientHandler(t, tc.list)
			if rec := post(h, tc.p, tc.body); rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
			if len(wm.starts) != 0 || len(repo.owners) != 0 {
				t.Errorf("starts = %v, owners = %v, want none", wm.starts, repo.owners)
			}
		})
	}
}

func TestClientHandlerBadRequests(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":        `{`,
		"no workflowId":   `{"flowId":"task-1"}`,
		"no flowId":       `{"workflowId":"notice"}`,
		"slash in flowId": `{"workflowId":"notice","flowId":"a/b"}`,
		"bad attributes":  `{"workflowId":"notice","flowId":"task-1","attributes":["Adam PVT LTD"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			h, _, wm := newClientHandler(t, clientFlows)
			if rec := post(h, tnsw, body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if len(wm.starts) != 0 {
				t.Errorf("starts = %v, want none", wm.starts)
			}
		})
	}
}

// A flow ID already started on another case is a conflict, and records no owner.
func TestClientHandlerConflict(t *testing.T) {
	repo, wm := newMemRepo(Flow{WorkflowID: "task-1", CaseID: "C2", TemplateID: "notice", Status: StatusStarted, ManifestVersion: 2}), &startCapture{}
	h := NewClientHandler(newService(t, repo, wm), clientFlows, catalogClients)
	if rec := post(h, tnsw, startBody); rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
	if len(repo.owners) != 0 {
		t.Errorf("owners = %v, want none", repo.owners)
	}
}

// A failed start leaves the flow recorded with its owner, so the caller's retry starts it.
// A failed record starts nothing.
func TestClientHandlerFailures(t *testing.T) {
	t.Run("start fails", func(t *testing.T) {
		repo, wm := newMemRepo(), &startCapture{err: errors.New("temporal down")}
		h := NewClientHandler(newService(t, repo, wm), clientFlows, catalogClients)
		if rec := post(h, tnsw, startBody); rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		wm.err = nil
		if rec := post(h, tnsw, startBody); rec.Code != http.StatusOK {
			t.Fatalf("retry status = %d, body %s", rec.Code, rec.Body)
		}
		if repo.flows["task-1"].Status != StatusStarted || len(repo.owners) != 1 || repo.owners[0] != "C1/officer/cda" {
			t.Errorf("flow = %+v, owners = %v, want started and owned by cda as officer", repo.flows["task-1"], repo.owners)
		}
	})
	t.Run("record fails", func(t *testing.T) {
		repo, wm := newMemRepo(), &startCapture{}
		repo.recordErr = errors.New("db down")
		h := NewClientHandler(newService(t, repo, wm), clientFlows, catalogClients)
		if rec := post(h, tnsw, startBody); rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		if len(wm.starts) != 0 || len(repo.owners) != 0 {
			t.Errorf("starts = %v, owners = %v, want none", wm.starts, repo.owners)
		}
	})
}
