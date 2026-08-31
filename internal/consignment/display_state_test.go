package consignment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTraderFacingState(t *testing.T) {
	reviewAskingForInfo := map[string]any{
		"reviewerform": map[string]any{"review_outcome": "needs_more_info"},
	}

	cases := []struct {
		name  string
		state string
		data  map[string]any
		want  WorkflowNodeState
	}{
		{name: "completed", state: "COMPLETED", want: WorkflowNodeStateCompleted},
		{name: "failed", state: "FAILED", want: WorkflowNodeStateFailed},
		{name: "empty falls back to in progress", state: "", want: WorkflowNodeStateInProgress},
		{name: "first fill stays pending user", state: "PENDING_USER", want: WorkflowNodeStatePendingUser},
		{
			name:  "userform value is ignored",
			state: "PENDING_USER",
			data:  map[string]any{"userform": map[string]any{"notes": "needs_more_info"}},
			want:  WorkflowNodeStatePendingUser,
		},
		{
			name:  "prior approve outcome is not pending feedback",
			state: "PENDING_USER",
			data:  map[string]any{"reviewerform": map[string]any{"review_outcome": "approve"}},
			want:  WorkflowNodeStatePendingUser,
		},
		{
			name:  "queued after submit is awaiting the OGA",
			state: "QUEUED_EXTERNALLY",
			data:  reviewAskingForInfo, // leftover from a previous round must not override
			want:  WorkflowNodeStateQueuedExternally,
		},
		{
			name:  "user-pending with needs_more_info is pending feedback",
			state: "PENDING_USER",
			data:  reviewAskingForInfo,
			want:  WorkflowNodeStatePendingFeedback,
		},
		{
			name:  "CDA verificationform needs_more_info is pending feedback",
			state: "PENDING_USER",
			data:  map[string]any{"verificationform": map[string]any{"verification_outcome": "needs_more_info"}},
			want:  WorkflowNodeStatePendingFeedback,
		},
		{
			name:  "FCAU application_review_outcome is pending feedback",
			state: "PENDING_USER",
			data: map[string]any{
				"reviewerform": map[string]any{"application_review_outcome": "needs_more_info"},
			},
			want: WorkflowNodeStatePendingFeedback,
		},
		{
			name:  "legacy in-progress with needs_more_info is pending feedback",
			state: "IN_PROGRESS",
			data:  reviewAskingForInfo,
			want:  WorkflowNodeStatePendingFeedback,
		},
		{
			name:  "lab fail_resubmit is pending feedback",
			state: "PENDING_USER",
			data:  map[string]any{"reviewerform": map[string]any{"lab_result": "fail_resubmit"}},
			want:  WorkflowNodeStatePendingFeedback,
		},
		{
			name:  "status override passes through",
			state: "PENDING_SLIP_UPLOAD",
			want:  WorkflowNodeState("PENDING_SLIP_UPLOAD"),
		},
		{
			name:  "nested slice still finds the outcome",
			state: "PENDING_USER",
			data:  map[string]any{"history": []any{map[string]any{"outcome": "needs_more_info"}}},
			want:  WorkflowNodeStatePendingFeedback,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, traderFacingState(tc.state, tc.data))
		})
	}
}
