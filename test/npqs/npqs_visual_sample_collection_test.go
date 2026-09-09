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

// TestVisualSampleCollectionSubWorkflow drives 4-0-visual_sample_collection's
// own workflow.json directly (not through the parent npqs_workflow.json,
// which the other tests in this package mock at too high a level to catch
// output_mapping bugs inside a nested sub-workflow). The officer_receive
// activity result mirrors the exact flat payload NSW Agency's
// ApplicationDetailScreen actually posts back (reviewerResponse, un-namespaced -
// see internal/application/service.go's ReviewApplication), so this test
// would have caught the "reviewerform.sample_number" output_mapping bug that
// left a real workflow instance permanently parked.
func TestVisualSampleCollectionSubWorkflow(t *testing.T) {
	bytes, err := os.ReadFile("../../../one-trade-artifacts/tnsw/npqs/4-0-visual_sample_collection/workflow.json")
	require.NoError(t, err)
	var def engine.WorkflowDefinition
	require.NoError(t, json.Unmarshal(bytes, &def))

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	acts := &engine.Activities{
		ExecuteTaskActivityHandler: func(p engine.TaskPayload) (map[string]any, error) {
			require.Equal(t, "npqs-visual-sample-collection--officer-receive", p.TaskTemplateID)
			// Flat, un-namespaced - exactly what CompleteTaskStep's payload
			// parameter carries into mapTaskOutputs (see
			// core/taskflow/orchestrator/manager.go CompleteTaskStep, which
			// passes the raw officer submission, not record.Data's
			// reviewerform-namespaced copy).
			return map[string]any{
				"sample_number":    "VSMP-2026-001",
				"receive_comments": "Sample bag intact, sealed correctly.",
				"review_outcome":   "approve",
			}, nil
		},
		WorkflowCompletedActivityHandler: func(workflowID string, finalContext map[string]any) error {
			return nil
		},
	}

	env.RegisterActivityWithOptions(acts.ExecuteTaskActivity, activity.RegisterOptions{Name: "ExecuteTaskActivity"})
	env.RegisterActivityWithOptions(acts.WorkflowCompletedActivity, activity.RegisterOptions{Name: "WorkflowCompletedActivity"})
	env.RegisterWorkflowWithOptions(engine.GraphInterpreterWorkflow, temporalworkflow.RegisterOptions{Name: "GraphInterpreterWorkflow"})
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: "visual-sample-collection-unit-test"})

	env.ExecuteWorkflow(engine.GraphInterpreterWorkflow, def, map[string]any{
		"reference_number": "NPQS/2026/06/01/ABC",
		"userform":         map[string]any{"nppo_office_location": "NPQS-KAT"},
		"commodities":      []any{map[string]any{"id": "item-1", "commodity_common_name": "Cut Rose Flowers"}},
	})

	require.True(t, env.IsWorkflowCompleted(), "sub-workflow must complete, not park, once officer_receive returns")
	require.NoError(t, env.GetWorkflowError())

	var result engine.WorkflowInstance
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "VSMP-2026-001", result.WorkflowVariables["sample_number"],
		"sample_number must flow from the flat officer_receive result into the sub-workflow's own output")
}
