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

// fakeLoader returns a canned template document, or an error.
type fakeLoader struct {
	doc string
	err error
}

func (l fakeLoader) GetTemplate(_ context.Context, _ string) ([]byte, error) {
	if l.err != nil {
		return nil, l.err
	}
	return []byte(l.doc), nil
}

func recordWith(data map[string]any) *store.TaskRecord {
	return &store.TaskRecord{TaskID: "task-1", Data: data}
}

func TestNotificationExtension_Execute(t *testing.T) {
	tests := []struct {
		name        string
		props       string
		payload     map[string]any
		record      *store.TaskRecord
		loaderDoc   string
		loaderErr   error
		sendErr     error
		devMode     bool
		wantErr     bool
		wantCalled  bool
		wantTo      string
		wantBody    string
		wantSubject string
		wantHTML    string
	}{
		{
			name:       "recipient resolved from payload notifyRecipient",
			props:      `{"channel":"sms","body":"received"}`,
			payload:    map[string]any{"notifyRecipient": "+94771234567"},
			record:     recordWith(nil),
			wantCalled: true,
			wantTo:     "+94771234567",
			wantBody:   "received",
		},
		{
			name:    "missing notifyRecipient errors",
			props:   `{"channel":"sms","body":"x"}`,
			record:  recordWith(nil),
			wantErr: true,
		},
		{
			name:    "missing notifyRecipient errors even in dev mode",
			props:   `{"channel":"sms","body":"x"}`,
			record:  recordWith(nil),
			devMode: true,
			wantErr: true,
		},
		{
			name:    "invalid request fails (empty body)",
			props:   `{"channel":"sms"}`,
			payload: map[string]any{"notifyRecipient": "+94771234567"},
			record:  recordWith(nil),
			wantErr: true,
		},
		{
			name:    "invalid request still errors in dev mode",
			props:   `{"channel":"sms"}`,
			payload: map[string]any{"notifyRecipient": "+94771234567"},
			record:  recordWith(nil),
			devMode: true,
			wantErr: true,
		},
		{
			name:       "send error surfaces when not dev mode",
			props:      `{"channel":"sms","body":"x"}`,
			payload:    map[string]any{"notifyRecipient": "+94771234567"},
			record:     recordWith(nil),
			sendErr:    errors.New("gateway down"),
			wantErr:    true,
			wantCalled: true,
		},
		{
			name:       "send error swallowed in dev mode",
			props:      `{"channel":"sms","body":"x"}`,
			payload:    map[string]any{"notifyRecipient": "+94771234567"},
			record:     recordWith(nil),
			sendErr:    errors.New("gateway down"),
			devMode:    true,
			wantCalled: true,
		},
		{
			name:        "template fields interpolate record.Data",
			props:       `{"channel":"email","template_id":"t"}`,
			payload:     map[string]any{"notifyRecipient": "a@b.lk"},
			loaderDoc:   `{"subject":"Hi {{.userform.name}}","body":"Ref {{.userform.ref}}","html_body":"<p>Hi {{.userform.name}}</p>"}`,
			record:      recordWith(map[string]any{"userform": map[string]any{"name": "Acme", "ref": "R-9"}}),
			wantCalled:  true,
			wantTo:      "a@b.lk",
			wantSubject: "Hi Acme",
			wantBody:    "Ref R-9",
			wantHTML:    "<p>Hi Acme</p>",
		},
		{
			name:      "missing template variable fails (non-dev)",
			props:     `{"channel":"sms","template_id":"t"}`,
			payload:   map[string]any{"notifyRecipient": "+94771234567"},
			loaderDoc: `{"body":"Hi {{.userform.missing}}"}`,
			record:    recordWith(map[string]any{"userform": map[string]any{"name": "Acme"}}),
			wantErr:   true,
		},
		{
			name:       "missing template variable swallowed in dev mode",
			props:      `{"channel":"sms","template_id":"t"}`,
			payload:    map[string]any{"notifyRecipient": "+94771234567"},
			loaderDoc:  `{"body":"Hi {{.userform.missing}}"}`,
			record:     recordWith(map[string]any{"userform": map[string]any{"name": "Acme"}}),
			devMode:    true,
			wantErr:    false,
			wantCalled: false,
		},
		{
			name:      "template_id not found errors",
			props:     `{"channel":"sms","template_id":"missing"}`,
			payload:   map[string]any{"notifyRecipient": "+94771234567"},
			loaderErr: errors.New("template \"missing\" not found"),
			record:    recordWith(nil),
			wantErr:   true,
		},
		{
			name:       "inline config falls back when template field empty",
			props:      `{"channel":"sms","template_id":"t","body":"inline-body"}`,
			payload:    map[string]any{"notifyRecipient": "+94771234567"},
			loaderDoc:  `{"subject":"only-subject"}`,
			record:     recordWith(nil),
			wantCalled: true,
			wantBody:   "inline-body",
		},
		{
			name:       "html_body escapes interpolated values",
			props:      `{"channel":"email","template_id":"t"}`,
			payload:    map[string]any{"notifyRecipient": "a@b.lk"},
			loaderDoc:  `{"html_body":"<p>{{.userform.name}}</p>"}`,
			record:     recordWith(map[string]any{"userform": map[string]any{"name": "<script>x</script>"}}),
			wantCalled: true,
			wantTo:     "a@b.lk",
			wantHTML:   "<p>&lt;script&gt;x&lt;/script&gt;</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeSender{err: tt.sendErr}
			ext := NewNotificationExtension(fs, fakeLoader{doc: tt.loaderDoc, err: tt.loaderErr}, tt.devMode)

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
			if tt.wantSubject != "" && fs.last.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", fs.last.Subject, tt.wantSubject)
			}
			if tt.wantHTML != "" && fs.last.HTMLBody != tt.wantHTML {
				t.Errorf("HTMLBody = %q, want %q", fs.last.HTMLBody, tt.wantHTML)
			}
		})
	}
}
