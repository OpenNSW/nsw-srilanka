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

	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/store"
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

// fakeClaimer records the claim calls that reach it and returns err, or claimErr or
// releaseErr from that call when set.
type fakeClaimer struct {
	err        error
	claimErr   error
	releaseErr error
	calls      []string // "claim:<task>:<holder>" or "release:<task>:<holder>"
}

func (f *fakeClaimer) ClaimTask(_ context.Context, taskID, holder string) error {
	f.calls = append(f.calls, "claim:"+taskID+":"+holder)
	if f.claimErr != nil {
		return f.claimErr
	}
	return f.err
}

func (f *fakeClaimer) ReleaseTask(_ context.Context, taskID, holder string) error {
	f.calls = append(f.calls, "release:"+taskID+":"+holder)
	if f.releaseErr != nil {
		return f.releaseErr
	}
	return f.err
}

// claimHandler is a handler over the pending HS-code task, with claims sent to claimer
// and audit events to auditor.
func claimHandler(t *testing.T, fetcher *fakeTaskFetcher, claimer *fakeClaimer, auditor *nswaudit.CaptureSink) *HTTPHandler {
	t.Helper()
	h := getTaskHandler(t, fetcher, &stubTemplates{})
	h.Claims = claimer
	h.ClaimsEnabled = true
	client := nswaudit.New()
	client.RegisterSink(auditor)
	h.Audit = client
	return h
}

// chaUser is a CHA user who owns the HS-code task's consignment, and so can read it,
// in a deployment that uses claims.
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
	claimer, auditor := &fakeClaimer{}, &nswaudit.CaptureSink{}
	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), claimer, auditor), opClaim, chaUser())

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"claim:" + testTaskID + ":" + testUserID}, claimer.calls)

	require.Len(t, auditor.Records(), 1)
	e := auditor.Records()[0]
	assert.Equal(t, nswaudit.ActionUpdate, e.Action)
	assert.Equal(t, nswaudit.TargetTask, e.TargetType)
	assert.Equal(t, nswaudit.StatusSuccess, e.Status)
	assert.Equal(t, "claim", e.Metadata["operation"])
	assert.Equal(t, testTaskID, e.TargetID)
}

func TestHandleReleaseTask_ReleasesForTheCaller(t *testing.T) {
	claimer, auditor := &fakeClaimer{}, &nswaudit.CaptureSink{}
	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), claimer, auditor), opRelease, chaUser())

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"release:" + testTaskID + ":" + testUserID}, claimer.calls)
	require.Len(t, auditor.Records(), 1)
	assert.Equal(t, "release", auditor.Records()[0].Metadata["operation"])
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
				recorder := sendClaim(claimHandler(t, tt.fetcher, claimer, &nswaudit.CaptureSink{}), op, tt.in)

				assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
				assert.Contains(t, recorder.Body.String(), tt.wantBody)
				assert.Empty(t, claimer.calls, "the task manager must not be reached")
			})
		}
	}
}

// Where the deployment does not use claims, claim and release are not found, and are
// answered before the task is read: they cannot be used to probe task ids, and nothing
// is claimed or audited.
func TestHandleClaimTask_OffWhereTheDeploymentDoesNotUseClaims(t *testing.T) {
	for _, op := range []string{opClaim, opRelease} {
		t.Run(op, func(t *testing.T) {
			fetcher, claimer, auditor := pendingHSCodeTask(), &fakeClaimer{}, &nswaudit.CaptureSink{}
			h := claimHandler(t, fetcher, claimer, auditor)
			h.ClaimsEnabled = false

			recorder := sendClaim(h, op, chaUser())

			assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), errClaimsOff)
			assert.Zero(t, fetcher.calls, "the task must not be read")
			assert.Empty(t, claimer.calls, "the task manager must not be reached")
			assert.Empty(t, auditor.Records())
		})
	}
}

// traderOnlyTask is the pending HS-code task with a read policy admitting traders only.
func traderOnlyTask() *fakeTaskFetcher {
	f := pendingHSCodeTask()
	f.renderConfig = `{"read":{"roles":["trader"]},"sections":{}}`
	return f
}

// A read denial is audited as a refused claim, like a denied task read.
func TestHandleClaimTask_ReadDenialIsAudited(t *testing.T) {
	notOwner := ownerInput("CHA", "trader")
	notOwner.UserID = testUserID
	auditor := &nswaudit.CaptureSink{}

	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), &fakeClaimer{}, auditor), opClaim, &notOwner)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Len(t, auditor.Records(), 1)
	assert.Equal(t, nswaudit.StatusFailure, auditor.Records()[0].Status)
	assert.Equal(t, "task read access denied", auditor.Records()[0].Metadata["error"])
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
		{"claim taken meanwhile", opClaim, &orchestrator.ClaimHeldError{Holder: "user-2"}, http.StatusConflict, errClaimChanged, true},
		{"claim on a completed task", opClaim, orchestrator.ErrTaskCompleted, http.StatusConflict, errClaimTaskComplete, true},
		{"release by another user", opRelease, orchestrator.ErrNotClaimHolder, http.StatusForbidden, errNotClaimHolder, true},
		{"task gone", opClaim, orchestrator.ErrTaskNotFound, http.StatusNotFound, errTaskNotFound, false},
		{"store failure", opRelease, errors.New("db down"), http.StatusInternalServerError, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditor := &nswaudit.CaptureSink{}
			recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), &fakeClaimer{err: tt.err}, auditor), tt.op, chaUser())

			assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
			assert.True(t, strings.Contains(recorder.Body.String(), tt.wantBody), "body = %s", recorder.Body.String())
			if tt.wantAudit {
				require.Len(t, auditor.Records(), 1)
				assert.Equal(t, nswaudit.StatusFailure, auditor.Records()[0].Status)
			} else {
				assert.Empty(t, auditor.Records())
			}
		})
	}
}

// --- takeover ---------------------------------------------------------------

// Claiming a task another user holds takes it over: their claim is released, then the
// caller claims it, and the audit trail records whose claim was taken over.
func TestHandleClaimTask_TakesOverAnotherUsersClaim(t *testing.T) {
	claimer, auditor := &fakeClaimer{}, &nswaudit.CaptureSink{}
	recorder := sendClaim(claimHandler(t, claimedHSCodeTask("user-2"), claimer, auditor), opClaim, chaUser())

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"release:" + testTaskID + ":user-2", "claim:" + testTaskID + ":" + testUserID}, claimer.calls)
	require.Len(t, auditor.Records(), 1)
	e := auditor.Records()[0]
	assert.Equal(t, nswaudit.StatusSuccess, e.Status)
	assert.Equal(t, "takeover", e.Metadata["operation"])
	assert.Equal(t, "user-2", e.Metadata["previous_holder"])
	assert.Equal(t, true, e.Metadata["previous_claim_released"])
}

// Claiming a task you already hold is a plain claim, not a takeover.
func TestHandleClaimTask_OwnClaimIsNotTakenOver(t *testing.T) {
	claimer, auditor := &fakeClaimer{}, &nswaudit.CaptureSink{}
	recorder := sendClaim(claimHandler(t, claimedHSCodeTask(testUserID), claimer, auditor), opClaim, chaUser())

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"claim:" + testTaskID + ":" + testUserID}, claimer.calls)
	require.Len(t, auditor.Records(), 1)
	assert.Equal(t, "claim", auditor.Records()[0].Metadata["operation"])
	assert.NotContains(t, auditor.Records()[0].Metadata, "previous_holder")
	assert.NotContains(t, auditor.Records()[0].Metadata, "previous_claim_released")
}

// A completed task keeps its claim as the record of who worked it: it is refused
// before the holder's claim could be released.
func TestHandleClaimTask_CompletedTaskIsNotTakenOver(t *testing.T) {
	fetcher := claimedHSCodeTask("user-2")
	fetcher.record.State = store.StateCompleted
	claimer, auditor := &fakeClaimer{}, &nswaudit.CaptureSink{}

	recorder := sendClaim(claimHandler(t, fetcher, claimer, auditor), opClaim, chaUser())

	assert.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), errClaimTaskComplete)
	assert.Empty(t, claimer.calls, "the task manager must not be reached")
	require.Len(t, auditor.Records(), 1)
	assert.Equal(t, nswaudit.StatusFailure, auditor.Records()[0].Status)
}

// When the claim or the task changes while a claim is being taken over, the takeover
// fails (409): the holder's claim was no longer theirs to release, or after the release
// someone else claimed the task or it completed. The audit trail says whether the
// previous claim was released, since a release that went through is not undone.
func TestHandleClaimTask_TakeoverRace(t *testing.T) {
	released := []string{"release:" + testTaskID + ":user-2", "claim:" + testTaskID + ":" + testUserID}
	tests := []struct {
		name         string
		claimer      *fakeClaimer
		wantBody     string
		wantCalls    []string
		wantReleased bool
	}{
		{
			"the claim moved before the release",
			&fakeClaimer{releaseErr: orchestrator.ErrNotClaimHolder},
			errClaimChanged,
			[]string{"release:" + testTaskID + ":user-2"},
			false,
		},
		{
			"someone claimed between the release and the claim",
			&fakeClaimer{claimErr: &orchestrator.ClaimHeldError{Holder: "user-3"}},
			errClaimChanged,
			released,
			true,
		},
		{
			"the task completed between the release and the claim",
			&fakeClaimer{claimErr: orchestrator.ErrTaskCompleted},
			errClaimTaskComplete,
			released,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditor := &nswaudit.CaptureSink{}
			recorder := sendClaim(claimHandler(t, claimedHSCodeTask("user-2"), tt.claimer, auditor), opClaim, chaUser())

			assert.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), tt.wantBody)
			assert.Equal(t, tt.wantCalls, tt.claimer.calls)
			require.Len(t, auditor.Records(), 1)
			e := auditor.Records()[0]
			assert.Equal(t, nswaudit.StatusFailure, e.Status)
			assert.Equal(t, "takeover", e.Metadata["operation"])
			assert.Equal(t, "user-2", e.Metadata["previous_holder"])
			assert.Equal(t, tt.wantReleased, e.Metadata["previous_claim_released"])
		})
	}
}

// --- tasks with a claim --------------------------------------------------------

// claimedHSCodeTask is the pending HS-code task, claimed by holder ("" for unclaimed).
func claimedHSCodeTask(holder string) *fakeTaskFetcher {
	f := pendingHSCodeTask()
	f.record.ClaimedBy = holder
	return f
}

// --- the claim on the task view ------------------------------------------------

// fakeUserService names users from a map, or fails with err.
type fakeUserService struct {
	names map[string]string
	err   error
}

func (f fakeUserService) GetDisplayName(_ context.Context, userID string) (string, error) {
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
	names := fakeUserService{names: map[string]string{testUserID: "me@example.com", "user-2": "other@example.com"}}

	tests := []struct {
		name       string
		holder     string
		in         *taskauthz.Input
		wantHolder *claimHolder
		wantMine   bool
	}{
		{"unclaimed", "", chaUser(), nil, false},
		{"claimed by the caller", testUserID, chaUser(), &claimHolder{Name: "me@example.com"}, true},
		{"claimed by another user", "user-2", chaUser(), &claimHolder{Name: "other@example.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := claimedHSCodeTask(tt.holder)
			if tt.holder != "" {
				fetcher.record.ClaimedAt = &at
			}
			h := getTaskHandler(t, fetcher, &stubTemplates{})
			h.ClaimsEnabled = true
			h.Users = names

			got := viewTask(t, h, tt.in).Claim
			require.NotNil(t, got)

			assert.Equal(t, tt.wantHolder, got.Holder)
			assert.Equal(t, tt.wantMine, got.Mine)
			if tt.holder != "" {
				require.NotNil(t, got.ClaimedAt)
				assert.True(t, got.ClaimedAt.Equal(at))
			} else {
				assert.Nil(t, got.ClaimedAt)
			}
		})
	}
}

// The view reports its claim once, under `claim`, and never the holder's user id: not
// in core's top-level claimed_by and claimed_at, nor on the holder.
func TestHandleGetTask_ClaimIsReportedOnlyUnderClaim(t *testing.T) {
	at := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	fetcher := claimedHSCodeTask("user-secret-2")
	fetcher.record.ClaimedAt = &at
	h := getTaskHandler(t, fetcher, &stubTemplates{})
	h.ClaimsEnabled = true
	h.Users = fakeUserService{names: map[string]string{"user-secret-2": "other@example.com"}}

	recorder := getTask(t, h, chaUser())

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Contains(t, body, "claim")
	assert.NotContains(t, body, "claimed_by")
	assert.NotContains(t, body, "claimed_at")
	assert.NotContains(t, recorder.Body.String(), "user-secret-2")
}

// The view is still served when the holder's name cannot be found, or there is no
// way to look it up: it says the task is claimed, by a holder with no name.
func TestHandleGetTask_ClaimHolderNameIsBestEffort(t *testing.T) {
	for name, users := range map[string]UserService{
		"lookup fails": fakeUserService{err: errors.New("db down")},
		"no lookup":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			h := getTaskHandler(t, claimedHSCodeTask("user-2"), &stubTemplates{})
			h.ClaimsEnabled = true
			h.Users = users

			got := viewTask(t, h, chaUser()).Claim
			require.NotNil(t, got)

			assert.Equal(t, &claimHolder{}, got.Holder)
		})
	}
}

// Where the deployment does not use claims, the task view carries no claim, not even
// a stored one, so a client shows no claim controls.
func TestHandleGetTask_NoClaimWhereClaimsAreOff(t *testing.T) {
	h := getTaskHandler(t, claimedHSCodeTask("user-2"), &stubTemplates{})

	recorder := getTask(t, h, chaUser())

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.NotContains(t, body, "claim")
	assert.NotContains(t, body, "claimed_by")
}

// The other user's id is not revealed in the conflict response.
func TestHandleClaimTask_ConflictDoesNotRevealTheHolderID(t *testing.T) {
	claimer := &fakeClaimer{err: &orchestrator.ClaimHeldError{Holder: "user-secret-2"}}
	recorder := sendClaim(claimHandler(t, pendingHSCodeTask(), claimer, &nswaudit.CaptureSink{}), opClaim, chaUser())

	require.Equal(t, http.StatusConflict, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "user-secret-2")
}
