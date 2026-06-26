// Package notify provides nsw-srilanka's task-completion notification
// extension (a side-effect that rides on another step's completion) together
// with the email + GovSMS channel providers it dispatches through the core
// notification.Manager.
//
// The providers mirror core's github.com/OpenNSW/core/notification/providers, but exist
// locally because that core package is pinned to the remote v0.1.0 auth API
// (auth.NewBearer(auth.BearerConfig{Token: string})), which is incompatible with
// remote v0.2.0 that nsw-srilanka requires for agency M2M client-credentials
// auth (configs/services*.json use env: secret refs, a v0.2.0 feature). The only
// substantive difference is the email provider's bearer construction, updated to
// v0.2.0's auth.NewBearer(token string).
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/remote/auth"
)

type emailConfig struct {
	BaseURL string `json:"baseURL"`
	Token   string `json:"token"`
}

type emailRequest struct {
	To       string `json:"to"`
	Subject  string `json:"subject,omitempty"`
	Body     string `json:"body,omitempty"`
	HTMLBody string `json:"htmlBody,omitempty"`
}

// EmailProvider sends email via an HTTP API using bearer token auth.
type EmailProvider struct {
	client *remote.Client
}

// NewEmailProvider returns a new EmailProvider ready for Configure.
func NewEmailProvider() *EmailProvider {
	return &EmailProvider{}
}

func (e *EmailProvider) Type() notification.ChannelType { return notification.ChannelEmail }

func (e *EmailProvider) Configure(raw json.RawMessage) error {
	var cfg emailConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("unmarshal email config: %w", err)
	}
	if cfg.BaseURL == "" {
		return errors.New("baseURL is required")
	}
	if err := validateBaseURL(cfg.BaseURL); err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("token is required")
	}
	// remote v0.2.0: NewBearer takes an already-resolved token string.
	e.client = remote.NewClient(cfg.BaseURL, remote.WithAuthenticator(auth.NewBearer(cfg.Token)))
	return nil
}

func (e *EmailProvider) Send(ctx context.Context, req notification.Request) error {
	if e.client == nil {
		return errors.New("email provider not configured")
	}
	if err := e.client.JSONRequest(ctx, remote.Request{
		Method: http.MethodPost,
		Path:   "/send",
		Body: emailRequest{
			To:       req.To,
			Subject:  req.Subject,
			Body:     req.Body,
			HTMLBody: req.HTMLBody,
		},
	}, nil); err != nil {
		return fmt.Errorf("email send: %w", err)
	}
	return nil
}

type smsConfig struct {
	BaseURL  string `json:"baseURL"`
	SIDCode  string `json:"sidCode"`
	UserName string `json:"userName"`
	Password string `json:"password"`
}

// SMSRequest matches the GovSMS V1 API envelope. Credentials are sent
// per-request in the body as required by the spec.
type SMSRequest struct {
	Data        string `json:"data"`
	PhoneNumber string `json:"phoneNumber"`
	SIDCode     string `json:"sIDCode"`
	UserName    string `json:"userName"`
	Password    string `json:"password"`
}

// SMSProvider sends SMS via the GovSMS service.
type SMSProvider struct {
	cfg    smsConfig
	client *remote.Client
}

// NewSMSProvider returns an SMSProvider ready for Configure.
func NewSMSProvider() *SMSProvider {
	return &SMSProvider{}
}

func (s *SMSProvider) Type() notification.ChannelType { return notification.ChannelSMS }

func (s *SMSProvider) Configure(raw json.RawMessage) error {
	if err := json.Unmarshal(raw, &s.cfg); err != nil {
		return fmt.Errorf("unmarshal sms config: %w", err)
	}
	if s.cfg.BaseURL == "" {
		return errors.New("baseURL is required")
	}
	if err := validateBaseURL(s.cfg.BaseURL); err != nil {
		return err
	}
	if s.cfg.SIDCode == "" {
		return errors.New("sidCode is required")
	}
	if s.cfg.UserName == "" {
		return errors.New("userName is required")
	}
	if s.cfg.Password == "" {
		return errors.New("password is required")
	}
	s.client = remote.NewClient(s.cfg.BaseURL)
	return nil
}

func (s *SMSProvider) Send(ctx context.Context, req notification.Request) error {
	if s.client == nil {
		return errors.New("sms provider not configured")
	}
	if err := s.client.JSONRequest(ctx, remote.Request{
		Method: http.MethodPost,
		Path:   "/send",
		Body: SMSRequest{
			Data:        req.Body,
			PhoneNumber: req.To,
			SIDCode:     s.cfg.SIDCode,
			UserName:    s.cfg.UserName,
			Password:    s.cfg.Password,
		},
	}, nil); err != nil {
		return fmt.Errorf("govsms send: %w", err)
	}
	return nil
}

// validateBaseURL ensures baseURL is absolute and uses HTTPS. Loopback hosts
// (localhost and the 127.0.0.0/8 and ::1 ranges) are exempt to support local
// development.
func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid baseURL: %w", err)
	}
	if u.Scheme == "" || u.Hostname() == "" {
		return errors.New("baseURL must be an absolute URL")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	isLoopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !isLoopback {
		return errors.New("baseURL must use HTTPS (except loopback hosts)")
	}
	return nil
}
