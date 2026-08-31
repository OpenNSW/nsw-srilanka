package consignment

// Review outcomes that send a task back to the trader. These are the values
// agency task-configs map to FEEDBACK_REQUESTED (see one-trade-artifacts
// taskconfig statusMap). Matching on the outcome — not an agency name — keeps
// the trader list badge generic across NPQS, FCAU, CDA, SLTB, and Customs.
var feedbackOutcomes = map[string]struct{}{
	"needs_more_info": {},
	"fail_resubmit":   {},
}

// traderFacingState maps a task orchestrator state onto the consignment-node
// state the trader UI displays.
//
// The orchestrator parks every non-terminal task as PENDING_USER (form) or
// QUEUED_EXTERNALLY (waiting on an OGA). Feedback loops reuse PENDING_USER, so
// without this mapping a modification request looks identical to first fill.
// When the parked user-input step still carries a prior review outcome that
// asked for more information, we surface PENDING_FEEDBACK instead.
func traderFacingState(state string, data map[string]any) WorkflowNodeState {
	switch state {
	case string(WorkflowNodeStateCompleted):
		return WorkflowNodeStateCompleted
	case string(WorkflowNodeStateFailed):
		return WorkflowNodeStateFailed
	case "":
		return WorkflowNodeStateInProgress
	}
	if isUserPending(state) && containsFeedbackOutcome(data) {
		return WorkflowNodeStatePendingFeedback
	}
	return WorkflowNodeState(state)
}

func isUserPending(state string) bool {
	return state == string(WorkflowNodeStatePendingUser) || state == string(WorkflowNodeStateInProgress)
}

// containsFeedbackOutcome walks task data for the agency review tokens that
// mean "send this back to the trader". The trader's own userform is skipped
// so a coincidental field value cannot flip the badge.
func containsFeedbackOutcome(data map[string]any) bool {
	if data == nil {
		return false
	}
	for k, child := range data {
		if k == "userform" {
			continue
		}
		if valueHasFeedbackOutcome(child) {
			return true
		}
	}
	return false
}

func valueHasFeedbackOutcome(v any) bool {
	switch t := v.(type) {
	case string:
		_, ok := feedbackOutcomes[t]
		return ok
	case map[string]any:
		for _, child := range t {
			if valueHasFeedbackOutcome(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if valueHasFeedbackOutcome(child) {
				return true
			}
		}
	}
	return false
}
