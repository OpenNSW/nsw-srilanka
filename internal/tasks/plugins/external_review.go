package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/OpenNSW/core/remote"
	coreplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"go.temporal.io/sdk/temporal"
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
	// Path is where a dispatch is POSTed. A reply ignores it: it always calls back
	// on /api/v1/callbacks/{replyToken}.
	Path     string `json:"path,omitempty"`
	TaskCode string `json:"task_code,omitempty"`
	// ReplyCommand is the command a reply carries, e.g. "submit" for a trader's
	// resubmission or "needs_more_info" for an officer asking for one. Required
	// only to reply.
	ReplyCommand string `json:"reply_command,omitempty"`
}

// replyTokenInput is the reserved input that makes the step reply instead of
// dispatch: the token the other side parked its own step on.
const replyTokenInput = "replyToken"

// Execute persists the reviewer form ID + QUEUED_EXTERNALLY status, then
// POSTs the submission to the OGA portal so the officer's review queue is
// populated. The body matches the SimpleFormExternalServiceRequest shape
// used by the legacy FCAU/NPQS OGA services.
//
// Given a replyToken input, it replies instead: the other side parked a step
// and handed over that token, so the step calls back on it (see reply) rather
// than dispatching a new review the other side would take for a retry.
func (p *ExternalReviewPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg externalReviewConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("external_review: invalid config: %w", err)
	}
	if cfg.ServiceID == "" {
		return fmt.Errorf("external_review: service_id is required")
	}
	replyToken, err := replyTokenFrom(ctx.Inputs)
	if err != nil {
		return err
	}
	if replyToken != "" {
		return p.reply(ctx, cfg, replyToken)
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

// reply completes the other side's parked step and parks this one in its place.
// It POSTs {command, payload} to /api/v1/callbacks/{replyToken}, the envelope
// every callback takes, with this step's own token as payload.callbackToken for
// the other side to answer on. The payload is the step's inputs as mapped, so the
// receiving step's output_mapping reads them by the names this node gave them.
//
// A failed reply fails the step like a failed dispatch does, and the engine
// retries it. A 409 is the exception: the token's step is no longer waiting, so no
// retry can succeed, and parking here would wait for an answer that can never
// come. It fails the step for good instead, which parks it for an admin.
func (p *ExternalReviewPlugin) reply(ctx pluginContext, cfg externalReviewConfig, replyToken string) error {
	if cfg.ReplyCommand == "" {
		return fmt.Errorf("external_review: reply_command is required to reply")
	}

	ctx.Record.State = "QUEUED_EXTERNALLY"

	callbackToken, err := coreplugins.CallbackToken(ctx.Record)
	if err != nil {
		return fmt.Errorf("external_review: %w", err)
	}
	payload := make(map[string]any, len(ctx.Inputs))
	for k, v := range ctx.Inputs {
		if k != replyTokenInput {
			payload[k] = v
		}
	}
	payload["callbackToken"] = callbackToken
	body := map[string]any{"command": cfg.ReplyCommand, "payload": payload}

	slog.Info("taskv2 external_review: replying on the other side's step",
		"taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID, "command", cfg.ReplyCommand)

	err = p.client.post(ctx.Context, cfg.ServiceID, "/api/v1/callbacks/"+url.PathEscape(replyToken), body)
	if remoteErr := (*remote.RemoteError)(nil); errors.As(err, &remoteErr) && remoteErr.StatusCode == http.StatusConflict {
		return temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("external_review: %s rejected the reply: its step is no longer waiting", cfg.ServiceID),
			"StaleReplyToken", err)
	}
	if err != nil {
		return err
	}
	return ErrSuspended
}

// replyTokenFrom returns the replyToken input, or "" when there is none. An empty
// value counts as none, so a workflow can clear the variable it maps from.
func replyTokenFrom(inputs map[string]any) (string, error) {
	v, ok := inputs[replyTokenInput]
	if !ok || v == nil {
		return "", nil
	}
	token, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("external_review: %s must be a string, got %T", replyTokenInput, v)
	}
	return token, nil
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
