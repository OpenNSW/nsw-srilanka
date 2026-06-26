package notify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/taskflow/store"
)

// fakeSender records the last request and optionally fails.
type fakeSender struct {
	last   notification.Request
	called bool
	err    error
}

func (f *fakeSender) Send(_ context.Context, req notification.Request) error {
	f.called = true
	f.last = req
	return f.err
}

func recordWith(data map[string]any) *store.TaskRecord {
	return &store.TaskRecord{TaskID: "task-1", Data: data}
}

func TestNotificationExtension_Execute(t *testing.T) {
	tests := []struct {
		name       string
		props      string
		payload    map[string]any
		record     *store.TaskRecord
		sendErr    error
		devMode    bool
		wantErr    bool
		wantCalled bool
		wantTo     string
		wantBody   string
	}{
		{
			name:       "recipient resolved from record.Data via to_path",
			props:      `{"channel":"sms","to_path":"applicant.phone","body":"received"}`,
			record:     recordWith(map[string]any{"applicant": map[string]any{"phone": "+94771234567"}}),
			wantCalled: true,
			wantTo:     "+94771234567",
			wantBody:   "received",
		},
		{
			name:       "payload to overrides to_path",
			props:      `{"channel":"sms","to_path":"applicant.phone","body":"received"}`,
			payload:    map[string]any{"to": "+94770000000"},
			record:     recordWith(map[string]any{"applicant": map[string]any{"phone": "+94771234567"}}),
			wantCalled: true,
			wantTo:     "+94770000000",
		},
		{
			name:       "payload overrides body and subject",
			props:      `{"channel":"email","to_path":"applicant.email","subject":"cfg","body":"cfg-body"}`,
			payload:    map[string]any{"subject": "form-subj", "body": "form-body"},
			record:     recordWith(map[string]any{"applicant": map[string]any{"email": "a@b.lk"}}),
			wantCalled: true,
			wantTo:     "a@b.lk",
			wantBody:   "form-body",
		},
		{
			name:       "static to fallback when payload and to_path absent",
			props:      `{"channel":"sms","to_path":"applicant.phone","to":"+94119999999","body":"ops"}`,
			record:     recordWith(map[string]any{}),
			wantCalled: true,
			wantTo:     "+94119999999",
		},
		{
			name:    "all recipient sources empty errors",
			props:   `{"channel":"sms","to_path":"applicant.phone","body":"x"}`,
			record:  recordWith(map[string]any{}),
			wantErr: true,
		},
		{
			name:    "invalid request fails (empty body)",
			props:   `{"channel":"sms","to":"+94771234567"}`,
			record:  recordWith(nil),
			wantErr: true,
		},
		{
			name:       "send error surfaces when not dev mode",
			props:      `{"channel":"sms","to":"+94771234567","body":"x"}`,
			record:     recordWith(nil),
			sendErr:    errors.New("gateway down"),
			wantErr:    true,
			wantCalled: true,
		},
		{
			name:       "send error swallowed in dev mode",
			props:      `{"channel":"sms","to":"+94771234567","body":"x"}`,
			record:     recordWith(nil),
			sendErr:    errors.New("gateway down"),
			devMode:    true,
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeSender{err: tt.sendErr}
			ext := NewNotificationExtension(fs, tt.devMode)

			err := ext.Execute(context.Background(), tt.record, tt.payload, json.RawMessage(tt.props))

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fs.called != tt.wantCalled {
				t.Fatalf("sender called = %v, want %v", fs.called, tt.wantCalled)
			}
			if tt.wantTo != "" && fs.last.To != tt.wantTo {
				t.Errorf("To = %q, want %q", fs.last.To, tt.wantTo)
			}
			if tt.wantBody != "" && fs.last.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", fs.last.Body, tt.wantBody)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	data := map[string]any{
		"applicant": map[string]any{
			"phone": "+94771234567",
			"empty": "",
			"num":   42,
		},
	}
	tests := []struct {
		name   string
		path   string
		want   string
		wantOk bool
	}{
		{"nested hit", "applicant.phone", "+94771234567", true},
		{"missing top key", "trader.phone", "", false},
		{"missing leaf key", "applicant.fax", "", false},
		{"non-string leaf", "applicant.num", "", false},
		{"empty string leaf", "applicant.empty", "", false},
		{"intermediate not a map", "applicant.phone.x", "", false},
		{"empty path", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolvePath(data, tt.path)
			if ok != tt.wantOk || got != tt.want {
				t.Errorf("resolvePath(%q) = (%q, %v), want (%q, %v)", tt.path, got, ok, tt.want, tt.wantOk)
			}
		})
	}
}
