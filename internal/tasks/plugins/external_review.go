package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/nsw/backend/pkg/remote"
)

// ExternalReviewPlugin is our custom replacement for the generic
// generic_external_review plugin. It supplies the OGA portal with a
// fully-populated submission envelope.
type ExternalReviewPlugin struct {
	client *dispatchHelper
}

// NewExternalReviewPlugin builds a plugin that POSTs the trader's submitted
// form to the configured service+path with a rich body shape.
func NewExternalReviewPlugin(manager *remote.Manager, backendBaseURL string, devMode bool) *ExternalReviewPlugin {
	return &ExternalReviewPlugin{client: newDispatchHelper(manager, backendBaseURL, devMode)}
}

type externalReviewConfig struct {
	ServiceID string `json:"service_id"`
	Path      string `json:"path"`
	TaskCode  string `json:"task_code,omitempty"`
}

// Execute persists the reviewer form ID + QUEUED_EXTERNALLY status, then
// POSTs the submission to the OGA portal so the officer's review queue is
// populated. The body matches the SimpleFormExternalServiceRequest shape
// used by the legacy FCAU/NPQS OGA services.
func (p *ExternalReviewPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg externalReviewConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("external_review: invalid config: %w", err)
	}
	if cfg.ServiceID == "" {
		return fmt.Errorf("external_review: service_id is required")
	}
	if cfg.Path == "" {
		return fmt.Errorf("external_review: path is required")
	}

	ctx.Record.State = "QUEUED_EXTERNALLY"

	// Convention: if input_mapping placed a value under the reserved key
	// "submission", that value is the wire shape OGA sees. Otherwise the
	// whole inputs bag is sent (default fallback for simple cases).
	var data any = ctx.Inputs
	if submission, ok := ctx.Inputs["submission"]; ok {
		data = submission
	}
	body := buildSubmissionBody(ctx.Record, data, &cfg.TaskCode, p.client.callbackTasksURL())

	slog.Info("taskv2 external_review: dispatching to OGA portal",
		"taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID, "path", cfg.Path, "taskCode", cfg.TaskCode)

	if err := p.client.post(ctx.Context, cfg.ServiceID, cfg.Path, body); err != nil {
		return err
	}
	return ErrSuspended
}

// buildSubmissionBody constructs the full envelope the OGA portal expects.
// data carries only the values declared by the workflow node's input_mapping
// — not the full record state — so the external reviewer sees the explicit
// contract surface and nothing more.
func buildSubmissionBody(record *store.TaskRecord, data any, taskCode *string, callbackURL string) map[string]any {
	if taskCode == nil || *taskCode == "" {
		taskCode = &record.ActiveTaskTemplateID
	}
	return map[string]any{
		"taskCode":      taskCode,
		"taskId":        record.TaskID,
		"consignmentId": rootWorkflowID(record.ParentWorkflowID),
		"serviceUrl":    callbackURL,
		"data":          data,
	}
}

// rootWorkflowID recovers the top-level consignment ID from a (possibly
// child-workflow-mangled) parent workflow ID by taking everything before the
// first "--" separator that FormatChildWorkflowID introduces for SPLIT_TASK
// branches (format: "{root}--{nodeID}--{branchID}").
//
// TODO: this is a stop-gap string-parsing derivation that duplicates the one
// in core/taskflow/store/gorm/model.go. Replace both once the engine threads a
// RootWorkflowID through TaskPayload/TaskRecord natively (see SPLIT_TASK /
// dynamic_split.go childVars propagation) so external dispatch can read
// record.RootWorkflowID directly instead of reconstructing it.
func rootWorkflowID(parentWorkflowID string) string {
	if idx := strings.Index(parentWorkflowID, "--"); idx != -1 {
		return parentWorkflowID[:idx]
	}
	return parentWorkflowID
}
