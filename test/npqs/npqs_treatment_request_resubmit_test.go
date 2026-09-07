package npqs_test

import (
	"encoding/json"
	"os"
	"testing"

	engine "github.com/OpenNSW/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

// TestTreatmentRequestSubWorkflow_ResubmitPrefillsPriorAnswer drives
// 5-1-treatment_request/workflow.json directly (the same "mock the leaf,
// catch sub-workflow wiring bugs the parent-level mock can't see" pattern
// as TestVisualSampleCollectionSubWorkflow) to prove the loop-back prefill
// fix: on a needs_more_info resubmit, trader_request's second invocation
// must be seeded with the trader's own PRIOR answer (as submitted the first
// time), not reset back to the pristine commodities the officer originally
// assessed.
func TestTreatmentRequestSubWorkflow_ResubmitPrefillsPriorAnswer(t *testing.T) {
	bytes, err := os.ReadFile("../../../one-trade-artifacts/tnsw/npqs_v2/5-1-treatment_request/workflow.json")
	require.NoError(t, err)
	var def engine.WorkflowDefinition
	require.NoError(t, json.Unmarshal(bytes, &def))

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	var traderRequestCalls int
	var officerReviewCalls int

	acts := &engine.Activities{
		ExecuteTaskActivityHandler: func(p engine.TaskPayload) (map[string]any, error) {
			switch p.TaskTemplateID {
			case "npqs-v2-treatment-request--trader-request":
				traderRequestCalls++
				traderinput, _ := p.Inputs["traderinput"].(map[string]any)
				prefill, _ := traderinput["treatment_items"].([]any)

				if traderRequestCalls == 1 {
					// First entry: seeded from the pristine, officer-assessed
					// commodities list (what the outer workflow provides at n1_apply).
					require.Len(t, prefill, 1)
					item, _ := prefill[0].(map[string]any)
					assert.Equal(t, "item-3", item["id"], "first entry must show the pristine commodities list")

					// Trader submits their treatment plan.
					return map[string]any{
						"treatment_items": []any{
							map[string]any{
								"id":                 "item-3",
								"treatment_type":     "FU",
								"chemical_used":      "Methyl Bromide",
								"treatment_duration": "24 hours",
							},
						},
						"general_remarks": "first submission",
					}, nil
				}

				// Second entry (resubmit after needs_more_info): must be prefilled
				// with the trader's OWN prior answer, not the pristine seed again.
				require.Len(t, prefill, 1, "resubmit must still be seeded, from the trader's own prior submission")
				item, _ := prefill[0].(map[string]any)
				assert.Equal(t, "item-3", item["id"])
				assert.Equal(t, "FU", item["treatment_type"], "resubmit must carry forward the trader's own prior treatment_type, not reset to blank")
				assert.Equal(t, "Methyl Bromide", item["chemical_used"], "resubmit must carry forward the trader's own prior chemical_used")

				return map[string]any{
					"treatment_items": []any{
						map[string]any{
							"id":                 "item-3",
							"treatment_type":     "FU",
							"chemical_used":      "Methyl Bromide",
							"treatment_duration": "48 hours", // trader corrects just the flagged field
						},
					},
					"general_remarks": "corrected duration per officer feedback",
				}, nil

			case "npqs-v2-treatment-request--officer-review":
				officerReviewCalls++
				if officerReviewCalls == 1 {
					return map[string]any{
						"treatment_request_outcome": "needs_more_info",
						"clarification_comments":    "please specify a longer fumigation duration",
					}, nil
				}
				return map[string]any{
					"treatment_request_outcome": "approve",
				}, nil

			default:
				return map[string]any{}, nil
			}
		},
		WorkflowCompletedActivityHandler: func(workflowID string, finalContext map[string]any) error {
			return nil
		},
	}

	env.RegisterActivityWithOptions(acts.ExecuteTaskActivity, activity.RegisterOptions{Name: "ExecuteTaskActivity"})
	env.RegisterActivityWithOptions(acts.WorkflowCompletedActivity, activity.RegisterOptions{Name: "WorkflowCompletedActivity"})
	env.RegisterWorkflowWithOptions(engine.GraphInterpreterWorkflow, temporalworkflow.RegisterOptions{Name: "GraphInterpreterWorkflow"})
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: "treatment-request-resubmit-unit-test"})

	// Mirrors what the outer npqs_workflow.json now seeds: "traderinput.treatment_items"
	// (the exact nested path trader_request's own output_mapping later writes back to)
	// holds the pristine per-item list on first entry.
	env.ExecuteWorkflow(engine.GraphInterpreterWorkflow, def, map[string]any{
		"reference_number": "NPQS/2026/06/01/ABC",
		"userform":         map[string]any{"applicant_name": "Lanka Agro Exports Ltd"},
		"traderinput": map[string]any{
			"treatment_items": []any{
				map[string]any{"id": "item-3", "commodity_common_name": "Sawn Timber"},
			},
		},
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	assert.Equal(t, 2, traderRequestCalls, "trader_request must run twice: initial submission + resubmit after needs_more_info")
	assert.Equal(t, 2, officerReviewCalls, "officer_review must run twice: needs_more_info then approve")

	var result engine.WorkflowInstance
	require.NoError(t, env.GetWorkflowResult(&result))
	reviewerform, _ := result.WorkflowVariables["reviewerform"].(map[string]any)
	assert.Equal(t, "approve", reviewerform["treatment_request_outcome"])
}
