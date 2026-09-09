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

// TestTreatmentUploadCertsSubWorkflow_PrefillsPriorSubmission drives
// 5-4-treatment_upload_certs/workflow.json directly. Unlike the treatment
// request loop (which loops WITHIN one sub-workflow instance), this loop
// re-enters the OUTER task node on needs_more_info, spawning a FRESH
// sub-workflow instance each time — the outer graph is what carries the
// prior submission forward (as npqs.treatment_traderinput), relayed in via
// prior_treatment_certificate_url etc. This test proves just the inner half
// of that relay: seeded with those prior_* fields (as the outer's
// input_mapping would provide them), trader_upload must surface them as
// traderinput.treatment_certificate_url etc. in the task's own input.
func TestTreatmentUploadCertsSubWorkflow_PrefillsPriorSubmission(t *testing.T) {
	bytes, err := os.ReadFile("../../../one-trade-artifacts/tnsw/npqs/5-4-treatment_upload_certs/workflow.json")
	require.NoError(t, err)
	var def engine.WorkflowDefinition
	require.NoError(t, json.Unmarshal(bytes, &def))

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	acts := &engine.Activities{
		ExecuteTaskActivityHandler: func(p engine.TaskPayload) (map[string]any, error) {
			require.Equal(t, "npqs-upload-treatment-certs--user-input", p.TaskTemplateID)
			traderinput, _ := p.Inputs["traderinput"].(map[string]any)
			assert.Equal(t, "https://nsw.gov.lk/storage/cert-old.pdf", traderinput["treatment_certificate_url"],
				"resubmit must be prefilled with the prior certificate URL, not blank")
			assert.Equal(t, "supervised, all clear", traderinput["upload_remarks"],
				"resubmit must be prefilled with the prior remarks")
			return map[string]any{
				"treatment_certificate_url": "https://nsw.gov.lk/storage/cert-corrected.pdf",
				"upload_remarks":            "supervised, all clear",
			}, nil
		},
		WorkflowCompletedActivityHandler: func(workflowID string, finalContext map[string]any) error {
			return nil
		},
	}

	env.RegisterActivityWithOptions(acts.ExecuteTaskActivity, activity.RegisterOptions{Name: "ExecuteTaskActivity"})
	env.RegisterActivityWithOptions(acts.WorkflowCompletedActivity, activity.RegisterOptions{Name: "WorkflowCompletedActivity"})
	env.RegisterWorkflowWithOptions(engine.GraphInterpreterWorkflow, temporalworkflow.RegisterOptions{Name: "GraphInterpreterWorkflow"})
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: "treatment-upload-certs-prefill-unit-test"})

	// Mirrors what the outer npqs_workflow.json's n3_4_treatment_upload_certs
	// input_mapping relays in from npqs.treatment_traderinput on a resubmit.
	env.ExecuteWorkflow(engine.GraphInterpreterWorkflow, def, map[string]any{
		"reference_number":                "NPQS/2026/06/01/ABC",
		"userform":                        map[string]any{"applicant_name": "Lanka Agro Exports Ltd"},
		"commodities":                     []any{map[string]any{"id": "item-3", "commodity_common_name": "Sawn Timber"}},
		"prior_treatment_certificate_url": "https://nsw.gov.lk/storage/cert-old.pdf",
		"prior_upload_remarks":            "supervised, all clear",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result engine.WorkflowInstance
	require.NoError(t, env.GetWorkflowResult(&result))
	traderinput, _ := result.WorkflowVariables["traderinput"].(map[string]any)
	assert.Equal(t, "https://nsw.gov.lk/storage/cert-corrected.pdf", traderinput["treatment_certificate_url"])
}
