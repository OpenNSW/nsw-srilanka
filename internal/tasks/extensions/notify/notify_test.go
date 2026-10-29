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
	calls  int
	last   notification.Request
	called bool
	err    error
}

func (f *fakeSender) Send(_ context.Context, req notification.Request) error {
	f.calls++
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

type notificationExecuteTestCase struct {
	name          string
	props         string
	payload       map[string]any
	record        *store.TaskRecord
	loaderDoc     string
	loaderErr     error
	sendErr       error
	wantErr       bool
	wantCalled    bool
	wantSendCalls int
	wantTo        string
	wantBody      string
	wantSubject   string
	wantHTML      string
}

// assertNotificationOutcome checks Execute's error/no-error contract and how
// many times the sender was called.
func assertNotificationOutcome(t *testing.T, tt notificationExecuteTestCase, fs *fakeSender, err error) {
	t.Helper()

	if gotErr := err != nil; gotErr != tt.wantErr {
		t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
	}
	if fs.called != tt.wantCalled {
		t.Fatalf("sender called = %v, want %v", fs.called, tt.wantCalled)
	}
	if tt.wantSendCalls != 0 && fs.calls != tt.wantSendCalls {
		t.Fatalf("sender calls = %d, want %d", fs.calls, tt.wantSendCalls)
	}
}

// assertNotificationRequest checks the fields of the request the sender was
// called with, when it was called.
func assertNotificationRequest(t *testing.T, tt notificationExecuteTestCase, fs *fakeSender) {
	t.Helper()

	if !fs.called {
		return
	}
	for _, field := range []struct {
		name, got, want string
	}{
		{"To", fs.last.To, tt.wantTo},
		{"Body", fs.last.Body, tt.wantBody},
		{"Subject", fs.last.Subject, tt.wantSubject},
		{"HTMLBody", fs.last.HTMLBody, tt.wantHTML},
	} {
		if field.want != "" && field.got != field.want {
			t.Errorf("%s = %q, want %q", field.name, field.got, field.want)
		}
	}
}

func TestNotificationExtension_Execute(t *testing.T) {
	tests := []notificationExecuteTestCase{
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
			name:   "missing notifyRecipient skips (no error, no send)",
			props:  `{"channel":"sms","body":"x"}`,
			record: recordWith(nil),
		},
		{
			name:    "invalid request always fails (empty body is a config bug, not best-effort)",
			props:   `{"channel":"sms"}`,
			payload: map[string]any{"notifyRecipient": "+94771234567"},
			record:  recordWith(nil),
			wantErr: true,
		},
		{
			name:          "send error is swallowed, not fatal",
			props:         `{"channel":"sms","body":"x"}`,
			payload:       map[string]any{"notifyRecipient": "+94771234567"},
			record:        recordWith(nil),
			sendErr:       errors.New("gateway down"),
			wantCalled:    true,
			wantSendCalls: 1,
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
			name:      "missing template variable is swallowed, not fatal",
			props:     `{"channel":"sms","template_id":"t"}`,
			payload:   map[string]any{"notifyRecipient": "+94771234567"},
			loaderDoc: `{"body":"Hi {{.userform.missing}}"}`,
			record:    recordWith(map[string]any{"userform": map[string]any{"name": "Acme"}}),
		},
		{
			name:      "template_id not found is swallowed, not fatal",
			props:     `{"channel":"sms","template_id":"missing"}`,
			payload:   map[string]any{"notifyRecipient": "+94771234567"},
			loaderErr: errors.New("template \"missing\" not found"),
			record:    recordWith(nil),
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
			ext := NewNotificationExtension(fs, fakeLoader{doc: tt.loaderDoc, err: tt.loaderErr})

			err := ext.Execute(context.Background(), tt.record, tt.payload, json.RawMessage(tt.props))

			assertNotificationOutcome(t, tt, fs, err)
			assertNotificationRequest(t, tt, fs)
		})
	}
}
