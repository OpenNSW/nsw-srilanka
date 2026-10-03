// Package taskconfig defines the agency's task_config artifact: what an injected
// taskCode runs and how it is presented. It follows the sibling nsw-agency service's
// taskconfig package, with one difference: that service's config *is* the task
// (forms, behavior, permissions), while here a workflow is, so a config names the
// workflow and carries only what the workflow cannot say about itself.
package taskconfig

import (
	"fmt"
	"strings"
)

// CurrentSchemaVersion is the only schemaVersion Validate accepts: the workflow-bound
// shape of one-trade-artifacts' schemas/taskconfig.v2.schema.json. Version 1 is
// nsw-agency's inline-forms shape, which this package does not read.
const CurrentSchemaVersion = 2

// TaskConfig is one task_config artifact, looked up by its manifest id (the taskCode
// an injecting caller sends). TaskCode is informational, as in nsw-agency.
type TaskConfig struct {
	SchemaVersion int    `json:"schemaVersion"`
	TaskCode      string `json:"taskCode"`
	// Workflow is the workflow artifact id started for this task code.
	Workflow string   `json:"workflow"`
	Meta     TaskMeta `json:"meta"`
}

// TaskMeta is display metadata for a task code.
type TaskMeta struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Category    string `json:"category,omitempty"`
}

// Validate checks the config is usable.
func (c *TaskConfig) Validate() error {
	if c.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("task config %q: unsupported schemaVersion %d (want %d)", c.TaskCode, c.SchemaVersion, CurrentSchemaVersion)
	}
	if strings.TrimSpace(c.Workflow) == "" {
		return fmt.Errorf("task config %q: workflow is required", c.TaskCode)
	}
	return nil
}
