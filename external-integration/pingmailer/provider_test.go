package pingmailer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	notification "github.com/OpenNSW/core/notifications"
	"github.com/OpenNSW/core/remote/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func config(baseURL string) Config {
	return Config{
		BaseURL:  baseURL,
		Auth:     auth.Config{Type: auth.TypeBearer, Bearer: &auth.BearerConfig{Token: "access-token"}},
		SMTPHost: "mail.example.com",
		SMTPPort: 587,
		Sender:   "no-reply@example.com",
	}
}

// An email is posted to /notify with the SMTP settings, one recipient and the
// content as written, carrying the client's access token.
func TestSend_PostsTheEmailToNotify(t *testing.T) {
	var path, authorization string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, authorization = r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	p, err := New(config(srv.URL))
	require.NoError(t, err)

	err = p.Send(context.Background(), notification.Request{
		Channel:  notification.ChannelEmail,
		To:       "trader@example.com",
		Subject:  "Tom & Jerry's application",
		Body:     "Dear <Name>, {{.not_a_template}}",
		HTMLBody: "<p>Dear Tom &amp; Jerry</p>",
	})

	require.NoError(t, err)
	assert.Equal(t, "/notify", path)
	assert.Equal(t, "Bearer access-token", authorization)
	assert.Equal(t, map[string]any{
		"smtp_host":     "mail.example.com",
		"smtp_port":     float64(587),
		"smtp_username": "no-reply@example.com",
		"smtp_sender":   "no-reply@example.com",
		"recipients":    []any{map[string]any{"email": "trader@example.com"}},
		"subject":       "Tom & Jerry's application",
		"text_body":     "Dear <Name>, {{.not_a_template}}",
		"html_body":     "<p>Dear Tom &amp; Jerry</p>",
	}, body)
}

// The sender's display name, when set, is PingMailer's smtp_username.
func TestSend_SenderName(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	cfg := config(srv.URL)
	cfg.SenderName = "Trade National Single Window"
	p, err := New(cfg)
	require.NoError(t, err)

	require.NoError(t, p.Send(context.Background(), notification.Request{Channel: notification.ChannelEmail, To: "a@example.com", Body: "hi"}))

	assert.Equal(t, "Trade National Single Window", body["smtp_username"])
	assert.Equal(t, "no-reply@example.com", body["smtp_sender"])
	assert.NotContains(t, body, "html_body", "an email without an HTML body sends none")
}

// A refusal from PingMailer is the send's error.
func TestSend_Refused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Missing required field: text_body or html_body", http.StatusBadRequest)
	}))
	defer srv.Close()
	p, err := New(config(srv.URL))
	require.NoError(t, err)

	assert.Error(t, p.Send(context.Background(), notification.Request{Channel: notification.ChannelEmail, To: "a@example.com", Body: "hi"}))
}

func TestValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		edit    func(*Config)
		wantErr string
	}{
		"valid":      {func(*Config) {}, ""},
		"https":      {func(c *Config) { c.BaseURL = "https://mail-api.example.com" }, ""},
		"plain http": {func(c *Config) { c.BaseURL = "http://api-server.mail.svc.cluster.local:8000" }, "must use HTTPS"},
		"plain http, allowed": {func(c *Config) {
			c.BaseURL, c.AllowInsecureHTTP = "http://api-server.mail.svc.cluster.local:8000", true
		}, ""},
		"no baseURL":            {func(c *Config) { c.BaseURL = "" }, "baseURL is required"},
		"relative baseURL":      {func(c *Config) { c.BaseURL = "/notify" }, "absolute"},
		"no auth":               {func(c *Config) { c.Auth = auth.Config{} }, "auth is required"},
		"no smtpHost":           {func(c *Config) { c.SMTPHost = "" }, "smtpHost is required"},
		"no smtpPort":           {func(c *Config) { c.SMTPPort = 0 }, "smtpPort"},
		"sender not an address": {func(c *Config) { c.Sender = "no-reply" }, "sender"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := config("http://localhost:8000")
			tc.edit(&cfg)
			err := cfg.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}
