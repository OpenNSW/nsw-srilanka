package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
	"github.com/OpenNSW/core/taskflow/plugins"
	workflow "github.com/OpenNSW/core/workflow"
	"go.temporal.io/sdk/temporal"
)

// workflowDefinitionLoader returns the definition handler the workflow engine
// calls to load a workflow by id from reg (RegisterDefinitionHandler). A
// workflow that isn't there or doesn't parse is returned as a non-retryable
// error, so the node that asked for it parks for an admin at once instead of
// retrying a load that can't succeed. The handler's error goes to Temporal as
// it is, so it carries the verdict itself; a loader I/O error stays retryable.
func workflowDefinitionLoader(reg *artifact.Registry) func(templateID string) (workflow.WorkflowDefinition, error) {
	return func(templateID string) (workflow.WorkflowDefinition, error) {
		def, err := workflowdef.Load(context.Background(), reg, templateID)
		if err == nil {
			return def, nil
		}
		err = fmt.Errorf("load workflow definition %q: %w", templateID, err)
		if errors.Is(err, artifact.ErrNotFound) || errors.Is(err, artifact.ErrInvalid) {
			return workflow.WorkflowDefinition{}, temporal.NewApplicationErrorWithOptions(
				err.Error(), plugins.ErrorTypeConfig, temporal.ApplicationErrorOptions{NonRetryable: true})
		}
		return workflow.WorkflowDefinition{}, err
	}
}
