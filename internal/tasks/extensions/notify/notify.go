package notify

import (
	"context"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"log/slog"
	"strings"
	texttemplate "text/template"

	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/taskflow/store"
)

// sender dispatches a notification request; satisfied by *notification.Manager.
type sender interface {
	Send(ctx context.Context, req notification.Request) error
}

// templateLoader fetches a notification template document (JSON) by id;
// satisfied by bootstrap's registryTemplateProvider.
type templateLoader interface {
	GetTemplate(ctx context.Context, id string) ([]byte, error)
}

// NotificationExtension fires an SMS or email when a workflow step completes.
// The recipient is taken from the completing step's payload under the
// "notifyRecipient" key. The send is a side-effect only and never mutates
// record. Transport failures are returned to the orchestrator (logged, never
// blocking); in devMode they are swallowed.
type NotificationExtension struct {
	sender  sender
	loader  templateLoader
	devMode bool
}

// NewNotificationExtension builds the extension. s and loader must be non-nil;
// Register enforces this.
func NewNotificationExtension(s sender, loader templateLoader, devMode bool) *NotificationExtension {
	return &NotificationExtension{sender: s, loader: loader, devMode: devMode}
}

type notificationConfig struct {
	Channel  string `json:"channel"`
	Subject  string `json:"subject,omitempty"`
	Body     string `json:"body,omitempty"`
	HTMLBody string `json:"html_body,omitempty"`
	// TemplateID names a generic_template artifact whose fields are rendered
	// against record.Data; a rendered field is used in preference to the inline
	// subject/body fallback (see Execute).
	TemplateID string `json:"template_id,omitempty"`
	TaskCode   string `json:"task_code,omitempty"`
}

// Execute resolves the recipient, builds and validates the request, and
// dispatches it. It is send-only and never mutates record.
func (e *NotificationExtension) Execute(ctx context.Context, record *store.TaskRecord, payload map[string]any, properties json.RawMessage) error {
	var cfg notificationConfig
	if err := json.Unmarshal(properties, &cfg); err != nil {
		return fmt.Errorf("notification: invalid properties: %w", err)
	}

	to, ok := stringLeaf(payload["notifyRecipient"])
	if !ok {
		return fmt.Errorf("notification: no recipient (payload[%q] empty)", "notifyRecipient")
	}

	var tmpl renderedTemplate
	if cfg.TemplateID != "" {
		var err error
		if tmpl, err = e.renderTemplate(ctx, cfg.TemplateID, record); err != nil {
			return e.swallowInDevMode(record, err)
		}
	}

	// Per-field precedence: rendered template → inline cfg.
	req := notification.Request{
		Channel:  notification.ChannelType(cfg.Channel),
		To:       to,
		Subject:  firstNonEmpty(tmpl.Subject, cfg.Subject),
		Body:     firstNonEmpty(tmpl.Body, cfg.Body),
		HTMLBody: firstNonEmpty(tmpl.HTMLBody, cfg.HTMLBody),
	}

	if err := req.Validate(); err != nil {
		return fmt.Errorf("notification: invalid request: %w", err)
	}

	slog.Info("notification extension: dispatching",
		"taskId", record.TaskID, "channel", req.Channel, "taskCode", cfg.TaskCode)

	if err := e.sender.Send(ctx, req); err != nil {
		return e.swallowInDevMode(record, fmt.Errorf("notification: send: %w", err))
	}

	slog.Info("notification extension: sent",
		"taskId", record.TaskID, "channel", req.Channel, "taskCode", cfg.TaskCode)
	return nil
}

// swallowInDevMode returns err unchanged in normal mode; in dev mode it logs and
// swallows it so a misconfigured template or flaky gateway never blocks local
// workflows.
func (e *NotificationExtension) swallowInDevMode(record *store.TaskRecord, err error) error {
	if !e.devMode {
		return err
	}
	slog.Warn("notification extension: error (dev mode — swallowing)",
		"taskId", record.TaskID, "error", err)
	return nil
}

// renderedTemplate holds the interpolated fields of a template document; a field
// is empty when the template omitted it.
type renderedTemplate struct {
	Subject  string
	Body     string
	HTMLBody string
}

// templateDoc is the on-disk shape of a generic_template notification document.
type templateDoc struct {
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	HTMLBody string `json:"html_body"`
}

// renderTemplate loads the template id and renders each non-empty field against
// record.Data. subject/body are plain text; html_body is HTML-escaped. An
// unknown variable (missingkey=error) is a render failure.
func (e *NotificationExtension) renderTemplate(ctx context.Context, id string, record *store.TaskRecord) (renderedTemplate, error) {
	if e.loader == nil {
		return renderedTemplate{}, fmt.Errorf("notification: template_id %q set but no template loader configured", id)
	}
	raw, err := e.loader.GetTemplate(ctx, id)
	if err != nil {
		return renderedTemplate{}, fmt.Errorf("notification: load template %q: %w", id, err)
	}
	var doc templateDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return renderedTemplate{}, fmt.Errorf("notification: invalid template %q: %w", id, err)
	}

	var data map[string]any
	if record != nil {
		data = record.Data
	}

	var out renderedTemplate
	if doc.Subject != "" {
		if out.Subject, err = renderText(doc.Subject, data); err != nil {
			return renderedTemplate{}, fmt.Errorf("notification: render template %q subject: %w", id, err)
		}
	}
	if doc.Body != "" {
		if out.Body, err = renderText(doc.Body, data); err != nil {
			return renderedTemplate{}, fmt.Errorf("notification: render template %q body: %w", id, err)
		}
	}
	if doc.HTMLBody != "" {
		if out.HTMLBody, err = renderHTML(doc.HTMLBody, data); err != nil {
			return renderedTemplate{}, fmt.Errorf("notification: render template %q html_body: %w", id, err)
		}
	}
	return out, nil
}

// renderText renders a plain-text template against data. missingkey=error makes
// an unknown variable a failure rather than emitting "<no value>".
func renderText(tmpl string, data any) (string, error) {
	t, err := texttemplate.New("notify").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return buf.String(), nil
}

// renderHTML renders an HTML body against data, auto-escaping interpolated
// values so workflow state cannot inject markup.
func renderHTML(tmpl string, data any) (string, error) {
	t, err := htmltemplate.New("notify").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return buf.String(), nil
}

// firstNonEmpty returns the first non-empty string in vals, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// stringLeaf returns v as a non-empty string when it is one.
func stringLeaf(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}
