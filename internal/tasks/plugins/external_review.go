package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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
	files  OGAFiles
}

// NewExternalReviewPlugin builds a plugin that POSTs the trader's submitted
// form to the configured service+path with a rich body shape. The files in it
// are sent as tokens for the OGA, issued through files.
func NewExternalReviewPlugin(manager *remote.Manager, backendBaseURL string, files OGAFiles) *ExternalReviewPlugin {
	return &ExternalReviewPlugin{client: newDispatchHelper(manager, backendBaseURL), files: files}
}

type externalReviewConfig struct {
	ServiceID string `json:"service_id"`
	// Path is where a dispatch is POSTed. A reply ignores it: it always calls back
	// on callbacksPath/{replyToken}.
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

// replyPayloadInput is the reserved input that, when present, is all a reply sends.
const replyPayloadInput = "payload"

// Execute persists the reviewer form ID + QUEUED_EXTERNALLY status, then
// POSTs the submission to the OGA portal so the officer's review queue is
// populated. The body matches the SimpleFormExternalServiceRequest shape
// used by the legacy FCAU/NPQS OGA services.
//
// Given a replyToken input, it replies instead: the other side parked a step
// and handed over that token, so the step calls back on it (see replyRequest)
// rather than dispatching a new review the other side would take for a retry.
//
// A failed call fails the step and the engine retries it. A 409 on a reply is the
// exception (see isConflict).
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
	callbackToken, err := coreplugins.CallbackToken(ctx.Record)
	if err != nil {
		return fmt.Errorf("external_review: %w", err)
	}

	var (
		path string
		body map[string]any
	)
	if replyToken != "" {
		path, body, err = p.reply(ctx, cfg, replyToken, callbackToken)
	} else {
		path, body, err = p.dispatchRequest(ctx, cfg, callbackToken)
	}
	if err != nil {
		return err
	}

	ctx.Record.State = "QUEUED_EXTERNALLY"
	slog.Info("taskv2 external_review: calling out",
		"taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID, "reply", replyToken != "")

	err = p.client.post(ctx.Context, cfg.ServiceID, path, body)
	if replyToken != "" && isConflict(err) {
		return temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("external_review: %s rejected the reply: its step is no longer waiting", cfg.ServiceID),
			"StaleReplyToken", err)
	}
	if err != nil {
		return err
	}
	return ErrSuspended
}

// dispatchRequest is the first round: the review the other side starts, POSTed to
// the configured path.
func (p *ExternalReviewPlugin) dispatchRequest(ctx pluginContext, cfg externalReviewConfig, callbackToken string) (string, map[string]any, error) {
	if cfg.Path == "" {
		return "", nil, fmt.Errorf("external_review: path is required")
	}

	// Convention: if input_mapping placed a value under the reserved key
	// "submission", that value is the wire shape OGA sees — write any
	// additional context directly into a nested "submission.<field>" path
	// in the node's own input_mapping (input_mapping's destination side
	// supports dot-paths natively). Otherwise the whole inputs bag is sent
	// (default fallback for simple cases).
	var data any = ctx.Inputs
	root := ""
	if submission, ok := ctx.Inputs["submission"]; ok {
		data, root = submission, "submission"
	}
	// The task's declared file fields are rooted at its data, which holds the
	// inputs; the OGA gets tokens for the ones it is sent.
	data, err := p.files.tokenize(ctx.Context, *ctx.Record, root, data, cfg.ServiceID)
	if err != nil {
		return "", nil, fmt.Errorf("external_review: %w", err)
	}
	return cfg.Path, buildSubmissionBody(ctx.Record, data, &cfg.TaskCode, callbackToken, p.client.callbacksURL()), nil
}

// reply is replyRequest for the step's inputs with their declared files sent as
// tokens for the other side's client, as a dispatch sends them. The inputs are
// the task's data, so the declared paths apply as they are.
func (p *ExternalReviewPlugin) reply(ctx pluginContext, cfg externalReviewConfig, replyToken, callbackToken string) (string, map[string]any, error) {
	// Only what the reply sends gets tokens, as a dispatch tokenizes only its
	// submission: the payload input when there is one, else all the inputs.
	root, data := "", any(ctx.Inputs)
	if payload, ok := ctx.Inputs[replyPayloadInput]; ok {
		if _, ok := payload.(map[string]any); !ok {
			// The inputs are the same on every retry, so none can succeed.
			return "", nil, temporal.NewNonRetryableApplicationError(
				fmt.Sprintf("external_review: %s must be an object, got %T", replyPayloadInput, payload),
				"InvalidReplyPayload", nil)
		}
		root, data = replyPayloadInput, payload
	}
	sent, err := p.files.tokenize(ctx.Context, *ctx.Record, root, data, cfg.ServiceID)
	if err != nil {
		return "", nil, fmt.Errorf("external_review: %w", err)
	}
	// tokenize can return data itself, which is the step's own, so the reply
	// gets a copy. All the inputs are non-nil here: they hold the replyToken.
	payload := maps.Clone(sent.(map[string]any))
	if root == "" {
		delete(payload, replyTokenInput)
	}
	return replyRequest(cfg, payload, replyToken, callbackToken)
}

// replyRequest is a callback on the other side's parked step: {command, payload} on
// replyToken, the envelope every callback takes, with this step's own token added to
// payload as callbackToken for the other side to answer on. The payload is the
// step's "payload" input when it has one, so that its other inputs are only for its
// own page to show, and all of its inputs but replyToken otherwise. The receiving
// step's output_mapping reads them by the names this node gave them.
func replyRequest(cfg externalReviewConfig, payload map[string]any, replyToken, callbackToken string) (string, map[string]any, error) {
	if cfg.ReplyCommand == "" {
		return "", nil, fmt.Errorf("external_review: reply_command is required to reply")
	}
	payload["callbackToken"] = callbackToken
	return callbacksPath + "/" + url.PathEscape(replyToken),
		map[string]any{"command": cfg.ReplyCommand, "payload": payload}, nil
}

// isConflict reports whether err is a 409: the token's step is no longer waiting, so
// no retry can succeed and parking here would wait for an answer that can never
// come. The step fails for good, which parks it for an admin.
func isConflict(err error) bool {
	var re *remote.RemoteError
	return errors.As(err, &re) && re.StatusCode == http.StatusConflict
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
		"consignmentId": record.CaseID,
		"serviceUrl":    callbackURL,
		"data":          data,
	}
}
