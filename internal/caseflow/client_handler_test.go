package caseflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// ownerCapture records each owner assigned.
type ownerCapture struct {
	assigned []string // "case/role/ou"
	err      error
}

func (o *ownerCapture) AssignOU(_ context.Context, caseID, role, ouHandle string) error {
	o.assigned = append(o.assigned, caseID+"/"+role+"/"+ouHandle)
	return o.err
}

// clientFlows lists notice for tnsw, owned by officer, in CDA's OU.
var clientFlows = ClientFlows{Enabled: true, OUHandle: "cda", Workflows: []ClientFlow{{ID: "notice", Clients: []string{"tnsw"}, Owner: "officer"}}}

// catalogClients is the catalog's Clients map: tnsw calls as NSW_TO_CDA.
var catalogClients = map[string]string{"tnsw": "NSW_TO_CDA", "fcau": "FCAU_TO_NSW"}

func newClientHandler(t *testing.T, list ClientFlows) (*ClientHandler, *memRepo, *startCapture, *ownerCapture) {
	t.Helper()
	repo, wm, owners := newMemRepo(), &startCapture{}, &ownerCapture{}
	return NewClientHandler(newService(t, repo, wm), list, owners, catalogClients), repo, wm, owners
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
	h, repo, wm, owners := newClientHandler(t, clientFlows)
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
	if len(owners.assigned) != 1 || owners.assigned[0] != "C1/officer/cda" {
		t.Errorf("owners = %v, want C1 owned by cda as officer", owners.assigned)
	}
}

// No callback token, no variable: a workflow that calls back by it fails at the start.
func TestClientHandlerWithoutCallbackToken(t *testing.T) {
	h, _, wm, _ := newClientHandler(t, clientFlows)
	if rec := post(h, tnsw, `{"workflowId":"notice","flowId":"task-1"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if _, ok := wm.starts[0].vars["callbackToken"]; ok {
		t.Error("callbackToken seeded without one in the request")
	}
}

// A repeat of the same request is answered from the recorded flow and records the same owner.
func TestClientHandlerRepeat(t *testing.T) {
	h, _, wm, owners := newClientHandler(t, clientFlows)
	for range 2 {
		if rec := post(h, tnsw, startBody); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	}
	if len(wm.starts) != 1 {
		t.Errorf("starts = %d, want 1", len(wm.starts))
	}
	if len(owners.assigned) != 2 || owners.assigned[0] != owners.assigned[1] {
		t.Errorf("owners = %v, want the same owner twice", owners.assigned)
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
			h, _, wm, owners := newClientHandler(t, tc.list)
			if rec := post(h, tc.p, tc.body); rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
			if len(wm.starts) != 0 || len(owners.assigned) != 0 {
				t.Errorf("starts = %v, owners = %v, want none", wm.starts, owners.assigned)
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
	} {
		t.Run(name, func(t *testing.T) {
			h, _, wm, _ := newClientHandler(t, clientFlows)
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
	repo, wm, owners := newMemRepo(Flow{WorkflowID: "task-1", CaseID: "C2", TemplateID: "notice", Status: StatusStarted, ManifestVersion: 2}), &startCapture{}, &ownerCapture{}
	h := NewClientHandler(newService(t, repo, wm), clientFlows, owners, catalogClients)
	if rec := post(h, tnsw, startBody); rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
	if len(owners.assigned) != 0 {
		t.Errorf("owners = %v, want none", owners.assigned)
	}
}

// A failed start records no owner; a failed owner record is an error the caller retries.
func TestClientHandlerFailures(t *testing.T) {
	t.Run("start fails", func(t *testing.T) {
		repo, wm, owners := newMemRepo(), &startCapture{err: errors.New("temporal down")}, &ownerCapture{}
		h := NewClientHandler(newService(t, repo, wm), clientFlows, owners, catalogClients)
		if rec := post(h, tnsw, startBody); rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		if len(owners.assigned) != 0 {
			t.Errorf("owners = %v, want none", owners.assigned)
		}
	})
	t.Run("owner record fails", func(t *testing.T) {
		repo, wm, owners := newMemRepo(), &startCapture{}, &ownerCapture{err: errors.New("db down")}
		h := NewClientHandler(newService(t, repo, wm), clientFlows, owners, catalogClients)
		if rec := post(h, tnsw, startBody); rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}
