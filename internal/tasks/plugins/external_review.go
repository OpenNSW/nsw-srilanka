package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/core/remote"
	coreplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
)

// ExternalReviewPlugin is our custom replacement for the generic
// generic_external_review plugin. It supplies the OGA portal with a
// fully-populated submission envelope.
type ExternalReviewPlugin struct {
	client *dispatchHelper
}

// NewExternalReviewPlugin builds a plugin that POSTs the trader's submitted
// form to the configured service+path with a rich body shape.
func NewExternalReviewPlugin(manager *remote.Manager, backendBaseURL string) *ExternalReviewPlugin {
	return &ExternalReviewPlugin{client: newDispatchHelper(manager, backendBaseURL)}
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
	// "submission", that value is the wire shape OGA sees — write any
	// additional context directly into a nested "submission.<field>" path
	// in the node's own input_mapping (input_mapping's destination side
	// supports dot-paths natively). Otherwise the whole inputs bag is sent
	// (default fallback for simple cases).
	var data any = ctx.Inputs
	if submission, ok := ctx.Inputs["submission"]; ok {
		data = submission
	}
	callbackToken, err := coreplugins.CallbackToken(ctx.Record)
	if err != nil {
		return fmt.Errorf("external_review: %w", err)
	}
	body := buildSubmissionBody(ctx.Record, data, &cfg.TaskCode, callbackToken, p.client.callbacksURL())

	slog.Info("taskv2 external_review: dispatching to OGA portal",
		"taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID, "path", cfg.Path, "taskCode", cfg.TaskCode)

	if err := p.client.post(ctx.Context, cfg.ServiceID, cfg.Path, body); err != nil {
		return err
	}
	return ErrSuspended
}

// buildSubmissionBody constructs the full envelope the OGA portal expects.
// callbackToken is opaque and names the step this dispatch is for; the reviewer
// calls back on {serviceUrl}/{callbackToken}, so a late or repeated callback
// can't complete a later step. It is also the reviewer's idempotency key: the
// same on every retry of this dispatch, different for every step.
//
// data carries only the values declared by the workflow node's input_mapping
// — not the full record state — so the external reviewer sees the explicit
// contract surface and nothing more.
func buildSubmissionBody(record *store.TaskRecord, data any, taskCode *string, callbackToken, callbackURL string) map[string]any {
	if taskCode == nil || *taskCode == "" {
		taskCode = &record.ActiveTaskTemplateID
	}
	return map[string]any{
		"taskCode":      taskCode,
		"taskId":        record.TaskID,
		"callbackToken": callbackToken,
		"consignmentId": record.RootWorkflowID,
		"serviceUrl":    callbackURL,
		"data":          data,
	}
}
