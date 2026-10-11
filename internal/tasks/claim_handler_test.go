package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUserID = "user-1"

// The two operations the claim routes offer: POST /claim and POST /release.
const (
	opClaim   = "claim"
	opRelease = "release"
)

// fakeClaimer records the claim calls that reach it and returns err.
type fakeClaimer struct {
	err   error
	calls []string // "claim:<task>:<holder>" or "release:<task>:<holder>"
}

func (f *fakeClaimer) ClaimTask(_ context.Context, taskID, holder string) error {
	f.calls = append(f.calls, "claim:"+taskID+":"+holder)
	return f.err
}

func (f *fakeClaimer) ReleaseTask(_ context.Context, taskID, holder string) error {
	f.calls = append(f.calls, "release:"+taskID+":"+holder)
	return f.err
}

// claimHandler is a handler over the pending HS-code task, with claims sent to claimer
// and audit events to auditor.
func claimHandler(t *testing.T, fetcher *fakeTaskFetcher, claimer *fakeClaimer, auditor *mockAuditor) *HTTPHandler {
	t.Helper()
	h := getTaskHandler(t, fetcher, &stubTemplates{})
	h.Claims = claimer
	h.Audit = nswaudit.NewRecorder(auditor)
	return h
}

// chaUser is a CHA user who owns the HS-code task's consignment, and so can read it.
func chaUser() *taskauthz.Input {
	in := ownerInput("CHA", "cha")
	in.UserID = testUserID
	return &in
}

// sendClaim sends a claim or a release (op is opClaim or opRelease) of the test task as in.
func sendClaim(h *HTTPHandler, op string, in *taskauthz.Input) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/"+op, nil)
	req.SetPathValue("id", testTaskID)
	if in != nil {
		req = req.WithContext(taskauthz.WithInput(req.Context(), *in))
	}
	recorder := httptest.NewRecorder()
	if op == opRelease {
		h.HandleReleaseTask(recorder, req)
	} else {
		h.HandleClaimTask(recorder, req)
	}
	return recorder
}

func TestHandleClaimTask_ClaimsForTheCaller(t *testing.T) {
	claimer, auditor := &fakeClaimer{}, &mockAuditor{}
	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), claimer, auditor), opClaim, chaUser())

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"claim:" + testTaskID + ":" + testUserID}, claimer.calls)

	require.Len(t, auditor.events, 1)
	e := auditor.events[0]
	assert.Equal(t, string(nswaudit.ActionUpdate), e.Action)
	assert.Equal(t, string(nswaudit.TargetTask), e.TargetType)
	assert.Equal(t, argus.StatusSuccess, e.Status)
	assert.Equal(t, "claim", e.Metadata["operation"])
	require.NotNil(t, e.TargetID)
	assert.Equal(t, testTaskID, *e.TargetID)
}

func TestHandleReleaseTask_ReleasesForTheCaller(t *testing.T) {
	claimer, auditor := &fakeClaimer{}, &mockAuditor{}
	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), claimer, auditor), opRelease, chaUser())

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"release:" + testTaskID + ":" + testUserID}, claimer.calls)
	require.Len(t, auditor.events, 1)
	assert.Equal(t, "release", auditor.events[0].Metadata["operation"])
}

// The claim is refused before it reaches the task manager when the caller is not a
// user who may read the task, and a read denial looks like a missing task.
func TestHandleClaimTask_RefusedBeforeTheManager(t *testing.T) {
	client := &taskauthz.Input{Kind: taskauthz.KindClient, ClientID: "FCAU_TO_NSW"}
	noUserID := ownerInput("CHA", "cha")
	notOwner := ownerInput("CHA", "trader") // a CHA token, but the company owns the consignment only as trader
	notOwner.UserID = testUserID
	// The trader-only read policy admits no CHA, so a CHA owner cannot read the task.
	deniedByPolicy := chaUser()

	tests := []struct {
		name       string
		fetcher    *fakeTaskFetcher
		in         *taskauthz.Input
		wantStatus int
		wantBody   string
	}{
		{"unauthenticated", pendingHSCodeTask(), nil, http.StatusUnauthorized, errAuthenticationReq},
		{"client", pendingHSCodeTask(), client, http.StatusForbidden, errClaimUsersOnly},
		{"user without a user id", pendingHSCodeTask(), &noUserID, http.StatusUnauthorized, errAuthenticationReq},
		{"missing task", &fakeTaskFetcher{}, chaUser(), http.StatusNotFound, errTaskNotFound},
		{"cannot read the task", pendingHSCodeTask(), &notOwner, http.StatusNotFound, errTaskNotFound},
		{"read policy excludes the role", traderOnlyTask(), deniedByPolicy, http.StatusNotFound, errTaskNotFound},
	}
	for _, tt := range tests {
		for _, op := range []string{opClaim, opRelease} {
			t.Run(tt.name+" "+op, func(t *testing.T) {
				claimer := &fakeClaimer{}
				recorder := sendClaim(claimHandler(t, tt.fetcher, claimer, &mockAuditor{}), op, tt.in)

				assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
				assert.Contains(t, recorder.Body.String(), tt.wantBody)
				assert.Empty(t, claimer.calls, "the task manager must not be reached")
			})
		}
	}
}

// traderOnlyTask is the pending HS-code task with a read policy admitting traders only.
func traderOnlyTask() *fakeTaskFetcher {
	f := pendingHSCodeTask()
	f.record.RenderConfig = []byte(`{"read":{"roles":["trader"]},"sections":{}}`)
	return f
}

// A read denial is audited as a refused claim, like a denied task read.
func TestHandleClaimTask_ReadDenialIsAudited(t *testing.T) {
	notOwner := ownerInput("CHA", "trader")
	notOwner.UserID = testUserID
	auditor := &mockAuditor{}

	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), &fakeClaimer{}, auditor), opClaim, &notOwner)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Len(t, auditor.events, 1)
	assert.Equal(t, argus.StatusFailure, auditor.events[0].Status)
	assert.Equal(t, "task read access denied", auditor.events[0].Metadata["error"])
}

// Each error the task manager returns is answered with its status, and the refusals
// (another user's claim, a completed task) are audited as failures.
func TestHandleClaimTask_MapsManagerErrors(t *testing.T) {
	tests := []struct {
		name       string
		op         string
		err        error
		wantStatus int
		wantBody   string
		wantAudit  bool
	}{
		{"claim held by another user", opClaim, &orchestrator.ClaimHeldError{Holder: "user-2"}, http.StatusConflict, errClaimHeld, true},
		{"claim on a completed task", opClaim, orchestrator.ErrTaskCompleted, http.StatusConflict, errClaimTaskComplete, true},
		{"release by another user", opRelease, orchestrator.ErrNotClaimHolder, http.StatusForbidden, errNotClaimHolder, true},
		{"task gone", opClaim, orchestrator.ErrTaskNotFound, http.StatusNotFound, errTaskNotFound, false},
		{"store failure", opRelease, errors.New("db down"), http.StatusInternalServerError, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditor := &mockAuditor{}
			recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), &fakeClaimer{err: tt.err}, auditor), tt.op, chaUser())

			assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
			assert.True(t, strings.Contains(recorder.Body.String(), tt.wantBody), "body = %s", recorder.Body.String())
			if tt.wantAudit {
				require.Len(t, auditor.events, 1)
				assert.Equal(t, argus.StatusFailure, auditor.events[0].Status)
			} else {
				assert.Empty(t, auditor.events)
			}
		})
	}
}

// --- the claim policy on step submissions ------------------------------------

// claimedHSCodeTask is the pending HS-code task, claimed by holder ("" for unclaimed).
func claimedHSCodeTask(holder string) *fakeTaskFetcher {
	f := pendingHSCodeTask()
	f.record.ClaimedBy = holder
	return f
}

// submitForClaimPolicy submits a step of the test task as in, through the portal route or,
// with byToken, the callback route. The body has no command: a submission the claim
// check lets through is then rejected by body validation (400) before it reaches the
// task manager, which these handlers do not have.
func submitForClaimPolicy(t *testing.T, h *HTTPHandler, in *taskauthz.Input, byToken bool) *httptest.ResponseRecorder {
	t.Helper()
	h.MaxRequestBytes = 1024
	recorder := httptest.NewRecorder()
	if byToken {
		token, err := callbacktoken.Encode("6aad0417-9a6d-4407-9509-2e51d8fcae99", "ca7ed707-1dba-43ca-94bf-10eddf00df3c")
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/"+token, strings.NewReader(`{}`))
		req.SetPathValue("token", token)
		if in != nil {
			req = req.WithContext(taskauthz.WithInput(req.Context(), *in))
		}
		h.HandleCompleteTaskStepByToken(recorder, req)
		return recorder
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-1", strings.NewReader(`{}`))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-1")
	if in != nil {
		req = req.WithContext(taskauthz.WithInput(req.Context(), *in))
	}
	h.HandleCompleteTaskStep(recorder, req)
	return recorder
}

// agencyUser is a user in a deployment that requires a claim to submit.
func agencyUser() *taskauthz.Input {
	in := chaUser()
	in.RequireClaimToSubmit = true
	return in
}

func TestSubmitStep_ClaimPolicy(t *testing.T) {
	client := &taskauthz.Input{Kind: taskauthz.KindClient, ClientID: "NSW_TO_CDA", RequireClaimToSubmit: true}

	tests := []struct {
		name       string
		fetcher    *fakeTaskFetcher
		in         *taskauthz.Input
		wantStatus int
		wantBody   string
	}{
		{"holder may submit", claimedHSCodeTask(testUserID), agencyUser(), http.StatusBadRequest, errInvalidRequestBody},
		{"unclaimed task is refused", claimedHSCodeTask(""), agencyUser(), http.StatusForbidden, errClaimRequired},
		{"another user's claim is refused", claimedHSCodeTask("user-2"), agencyUser(), http.StatusForbidden, errNotClaimHolder},
		{"missing task", &fakeTaskFetcher{}, agencyUser(), http.StatusNotFound, errTaskNotFound},
		{"no claim needed where the deployment does not require one", claimedHSCodeTask(""), chaUser(), http.StatusBadRequest, errInvalidRequestBody},
		{"a client never needs a claim", claimedHSCodeTask("user-2"), client, http.StatusBadRequest, errInvalidRequestBody},
	}
	for _, tt := range tests {
		for _, byToken := range []bool{false, true} {
			name := tt.name + " (portal route)"
			if byToken {
				name = tt.name + " (callback route)"
			}
			t.Run(name, func(t *testing.T) {
				recorder := submitForClaimPolicy(t, getTaskHandler(t, tt.fetcher, &stubTemplates{}), tt.in, byToken)

				assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
				assert.Contains(t, recorder.Body.String(), tt.wantBody)
			})
		}
	}
}

// A user with no internal user id never matches a claim, not even an unclaimed task's
// empty holder.
func TestSubmitStep_UserWithoutUserIDNeverHoldsTheClaim(t *testing.T) {
	in := agencyUser()
	in.UserID = ""

	recorder := submitForClaimPolicy(t, getTaskHandler(t, claimedHSCodeTask(""), &stubTemplates{}), in, false)

	assert.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
}

// Where no claim is required the task is not even read: the policy check costs
// nothing in TNSW.
func TestSubmitStep_NoLookupWhenNoClaimIsRequired(t *testing.T) {
	fetcher := claimedHSCodeTask("")
	submitForClaimPolicy(t, getTaskHandler(t, fetcher, &stubTemplates{}), chaUser(), false)

	assert.Zero(t, fetcher.calls)
}

// --- the claim on the task view ------------------------------------------------

// fakeNames names users from a map, or fails with err.
type fakeNames struct {
	names map[string]string
	err   error
}

func (f fakeNames) HolderName(_ context.Context, userID string) (string, error) {
	return f.names[userID], f.err
}

// viewTask reads the test task as in and decodes the response.
func viewTask(t *testing.T, h *HTTPHandler, in *taskauthz.Input) taskResponse {
	t.Helper()
	recorder := getTask(t, h, in)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var resp taskResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp), recorder.Body.String())
	return resp
}

func TestHandleGetTask_ShowsTheClaim(t *testing.T) {
	at := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	names := fakeNames{names: map[string]string{testUserID: "me@example.com", "user-2": "other@example.com"}}

	tests := []struct {
		name         string
		holder       string
		in           *taskauthz.Input
		wantHolder   *claimHolder
		wantMine     bool
		wantRequired bool
	}{
		{"unclaimed", "", chaUser(), nil, false, false},
		{"claimed by the caller", testUserID, chaUser(), &claimHolder{ID: testUserID, Name: "me@example.com"}, true, false},
		{"claimed by another user", "user-2", chaUser(), &claimHolder{ID: "user-2", Name: "other@example.com"}, false, false},
		{"claim required to submit", "", agencyUser(), nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := claimedHSCodeTask(tt.holder)
			if tt.holder != "" {
				fetcher.record.ClaimedAt = &at
			}
			h := getTaskHandler(t, fetcher, &stubTemplates{})
			h.Users = names

			got := viewTask(t, h, tt.in).Claim

			assert.Equal(t, tt.wantHolder, got.Holder)
			assert.Equal(t, tt.wantMine, got.Mine)
			assert.Equal(t, tt.wantRequired, got.RequiredToSubmit)
			if tt.holder != "" {
				require.NotNil(t, got.ClaimedAt)
				assert.True(t, got.ClaimedAt.Equal(at))
			} else {
				assert.Nil(t, got.ClaimedAt)
			}
		})
	}
}

// The view is still served when the holder's name cannot be found, or there is no
// way to look it up: it says who holds the claim by id.
func TestHandleGetTask_ClaimHolderNameIsBestEffort(t *testing.T) {
	for name, users := range map[string]HolderNames{
		"lookup fails": fakeNames{err: errors.New("db down")},
		"no lookup":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			h := getTaskHandler(t, claimedHSCodeTask("user-2"), &stubTemplates{})
			h.Users = users

			got := viewTask(t, h, chaUser()).Claim

			assert.Equal(t, &claimHolder{ID: "user-2"}, got.Holder)
		})
	}
}

// claimGatedRenderConfig shows one section per claim fact, so the view shows which
// fact was true for the caller.
const claimGatedRenderConfig = `{
  "read": { "roles": ["cha"] },
  "sections": {
    "mine":  { "templateId": "t", "projector": "MARKDOWN", "visibleWhen": { "requireClaim": "claim:mine" } },
    "other": { "templateId": "t", "projector": "MARKDOWN", "visibleWhen": { "requireClaim": "claim:other" } },
    "none":  { "templateId": "t", "projector": "MARKDOWN", "visibleWhen": { "requireClaim": "claim:none" } }
  }
}`

// Exactly one claim fact is true, so a render config can show each caller the
// section that fits who holds the claim.
func TestHandleGetTask_ClaimFactsGateSections(t *testing.T) {
	for holder, want := range map[string]string{"": "none", testUserID: "mine", "user-2": "other"} {
		t.Run(want, func(t *testing.T) {
			fetcher := claimedHSCodeTask(holder)
			fetcher.record.RenderConfig = json.RawMessage(claimGatedRenderConfig)

			resp := viewTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), chaUser())

			view := decodeSlots(t, resp.ZoneView)
			assert.Len(t, view, 1, "view = %v", view)
			assert.Contains(t, view, want)
		})
	}
}

// The other user's id is not revealed in the conflict response.
func TestHandleClaimTask_ConflictDoesNotRevealTheHolderID(t *testing.T) {
	claimer := &fakeClaimer{err: &orchestrator.ClaimHeldError{Holder: "user-secret-2"}}
	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), claimer, &mockAuditor{}), opClaim, chaUser())

	require.Equal(t, http.StatusConflict, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "user-secret-2")
}
