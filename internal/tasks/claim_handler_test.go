package tasks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	argus "github.com/LSFLK/argus/pkg/audit"
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

// The other user's id is not revealed in the conflict response.
func TestHandleClaimTask_ConflictDoesNotRevealTheHolderID(t *testing.T) {
	claimer := &fakeClaimer{err: &orchestrator.ClaimHeldError{Holder: "user-secret-2"}}
	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), claimer, &mockAuditor{}), opClaim, chaUser())

	require.Equal(t, http.StatusConflict, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "user-secret-2")
}
