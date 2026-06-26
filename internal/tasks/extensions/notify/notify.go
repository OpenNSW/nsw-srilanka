package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/taskflow/store"
)

// sender dispatches a notification request. It is satisfied by
// *notification.Manager; declaring it as an interface keeps
// NotificationExtension unit-testable without a live SMS/email provider.
type sender interface {
	Send(ctx context.Context, req notification.Request) error
}

// NotificationExtension fires an SMS or email as a side-effect of a workflow
// step completing. Extensions receive no input_mapping (the orchestrator passes
// only record, payload, and static properties), so the recipient is resolved
// from accumulated workflow state (record.Data) via a configured dotted path,
// with overrides from the completing step's payload and a static fallback.
//
// The send is a pure side-effect: extensions run against a deep copy of the
// record, so any writes are discarded — this extension never mutates record.
// Wired at POST_RESUME, transport failures are logged by the orchestrator and
// never block task completion; in devMode they are swallowed entirely.
type NotificationExtension struct {
	sender  sender
	devMode bool
}

// NewNotificationExtension builds the extension. s must be non-nil; bootstrap
// fail-fasts if the notification manager could not initialize.
func NewNotificationExtension(s sender, devMode bool) *NotificationExtension {
	return &NotificationExtension{sender: s, devMode: devMode}
}

type notificationConfig struct {
	Channel  string `json:"channel"`
	Subject  string `json:"subject,omitempty"`
	Body     string `json:"body,omitempty"`
	HTMLBody string `json:"html_body,omitempty"`
	TaskCode string `json:"task_code,omitempty"`
	// ToPath is a dotted path into record.Data (e.g. "applicant.phone")
	// locating the recipient captured by an earlier subtask. This is the
	// substitute for the input_mapping extensions do not get.
	ToPath string `json:"to_path,omitempty"`
	// To is a static recipient fallback (e.g. an internal ops desk) used only
	// when neither payload["to"] nor ToPath resolves.
	To string `json:"to,omitempty"`
}

// Execute resolves the recipient, builds the request from payload-or-properties,
// validates it, and dispatches via the manager. It is send-only and never
// mutates record (orchestrator discards extension writes).
func (e *NotificationExtension) Execute(ctx context.Context, record *store.TaskRecord, payload map[string]any, properties json.RawMessage) error {
	var cfg notificationConfig
	if err := json.Unmarshal(properties, &cfg); err != nil {
		return fmt.Errorf("notification: invalid properties: %w", err)
	}

	to, err := resolveRecipient(payload, record, cfg)
	if err != nil {
		return err
	}

	req := notification.Request{
		Channel:  notification.ChannelType(cfg.Channel),
		To:       to,
		Subject:  pickString(payload, "subject", cfg.Subject),
		Body:     pickString(payload, "body", cfg.Body),
		HTMLBody: pickString(payload, "html_body", cfg.HTMLBody),
	}

	if err := req.Validate(); err != nil {
		return fmt.Errorf("notification: invalid request: %w", err)
	}

	slog.Info("notification extension: dispatching",
		"taskId", record.TaskID, "channel", req.Channel, "taskCode", cfg.TaskCode)

	if err := e.sender.Send(ctx, req); err != nil {
		if !e.devMode {
			return fmt.Errorf("notification: send: %w", err)
		}
		slog.Warn("notification extension: send failed (dev mode — swallowing)",
			"taskId", record.TaskID, "channel", req.Channel, "error", err)
		return nil
	}

	slog.Info("notification extension: sent",
		"taskId", record.TaskID, "channel", req.Channel, "taskCode", cfg.TaskCode)
	return nil
}

// resolveRecipient picks the recipient in priority order: the completing step's
// payload["to"] (form override), then record.Data walked by cfg.ToPath (the
// normal per-trader case), then the static cfg.To fallback. Errors if all are
// empty — without a recipient there is nothing to send.
func resolveRecipient(payload map[string]any, record *store.TaskRecord, cfg notificationConfig) (string, error) {
	if to, ok := stringLeaf(payload["to"]); ok {
		return to, nil
	}
	if cfg.ToPath != "" && record != nil {
		if to, ok := resolvePath(record.Data, cfg.ToPath); ok {
			return to, nil
		}
	}
	if cfg.To != "" {
		return cfg.To, nil
	}
	return "", fmt.Errorf("notification: no recipient (payload[\"to\"], to_path %q, and static to all empty)", cfg.ToPath)
}

// resolvePath walks nested map[string]any state by a dotted path and returns the
// leaf as a non-empty string. It returns ok=false on any missing key, a
// non-map intermediate, or a non-string / empty leaf.
func resolvePath(data map[string]any, dotted string) (string, bool) {
	if dotted == "" || data == nil {
		return "", false
	}
	parts := strings.Split(dotted, ".")
	var cur any = data
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = m[p]
		if !ok {
			return "", false
		}
	}
	return stringLeaf(cur)
}

// stringLeaf returns v as a non-empty string when it is one.
func stringLeaf(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// pickString returns m[key] when present as a non-empty string, else fallback.
func pickString(m map[string]any, key, fallback string) string {
	if s, ok := stringLeaf(m[key]); ok {
		return s
	}
	return fallback
}
