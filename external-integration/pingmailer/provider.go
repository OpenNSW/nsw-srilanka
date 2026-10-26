// Package pingmailer sends email notifications through PingMailer's API
// (POST /notify). PingMailer relays each message to SMTP, authenticating with
// the access token the call carries (XOAUTH2), so the provider holds an OAuth2
// client and sends a live token, refreshed as it expires.
package pingmailer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/url"

	notification "github.com/OpenNSW/core/notifications"
	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/remote/auth"
)

// Config is the PingMailer provider's configuration, under
// notification.providers.email.pingmailer.
type Config struct {
	// BaseURL is PingMailer's API, for example
	// https://api-server.mail.svc.cluster.local:8000.
	BaseURL string `yaml:"baseURL"`
	// AllowInsecureHTTP permits a plain-HTTP BaseURL on a host other than
	// loopback, such as an in-cluster service. The access token travels on
	// that call, so it is off by default.
	AllowInsecureHTTP bool `yaml:"allowInsecureHTTP"`
	// Auth gets the token PingMailer hands to SMTP: an oauth2 client allowed to
	// send as Sender.
	Auth auth.Config `yaml:"auth"`
	// SMTPHost and SMTPPort are the SMTP server PingMailer relays to.
	SMTPHost string `yaml:"smtpHost"`
	SMTPPort int    `yaml:"smtpPort"`
	// Sender is the address every email is sent from, and the SMTP identity
	// the token is presented for.
	Sender string `yaml:"sender"`
	// SenderName is the From header's display name. Optional: the From header
	// is the bare Sender without it.
	SenderName string `yaml:"senderName"`
}

// Validate reports a missing or invalid setting.
func (c Config) Validate() error {
	if c.BaseURL == "" {
		return errors.New("pingmailer: baseURL is required")
	}
	if err := validateBaseURL(c.BaseURL, c.AllowInsecureHTTP); err != nil {
		return fmt.Errorf("pingmailer: %w", err)
	}
	if c.Auth.Type == "" {
		return errors.New("pingmailer: auth is required")
	}
	if c.SMTPHost == "" {
		return errors.New("pingmailer: smtpHost is required")
	}
	if c.SMTPPort <= 0 || c.SMTPPort > 65535 {
		return errors.New("pingmailer: smtpPort must be between 1 and 65535")
	}
	if _, err := mail.ParseAddress(c.Sender); err != nil {
		return fmt.Errorf("pingmailer: sender: %w", err)
	}
	return nil
}

// Provider sends email notifications through PingMailer.
type Provider struct {
	client *remote.Client
	cfg    Config
}

// New validates cfg and returns a provider ready to send.
func New(cfg Config) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	authenticator, err := auth.Build(cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("pingmailer: auth: %w", err)
	}
	return &Provider{client: remote.NewClient(cfg.BaseURL, remote.WithAuthenticator(authenticator)), cfg: cfg}, nil
}

func (p *Provider) Type() notification.ChannelType { return notification.ChannelEmail }

type recipient struct {
	Email string `json:"email"`
}

// notifyRequest is PingMailer's POST /notify body with the content as written:
// PingMailer applies no template to subject, text_body and html_body.
type notifyRequest struct {
	SMTPHost     string      `json:"smtp_host"`
	SMTPPort     int         `json:"smtp_port"`
	SMTPUsername string      `json:"smtp_username"`
	SMTPSender   string      `json:"smtp_sender"`
	Recipients   []recipient `json:"recipients"`
	Subject      string      `json:"subject"`
	TextBody     string      `json:"text_body,omitempty"`
	HTMLBody     string      `json:"html_body,omitempty"`
}

// Send hands the email to PingMailer. PingMailer answers 202 once it has
// queued the email and talks to SMTP afterwards, so a nil error means the
// email was accepted, not delivered: a delivery failure shows only in
// PingMailer's logs.
func (p *Provider) Send(ctx context.Context, req notification.Request) error {
	// PingMailer's smtp_username is the From header's display name; the
	// sender address is the From address and the SMTP identity.
	name := p.cfg.SenderName
	if name == "" {
		name = p.cfg.Sender
	}
	if err := p.client.Request(ctx, remote.Request{
		Method: http.MethodPost,
		Path:   "/notify",
		Body: remote.JSONBody{V: notifyRequest{
			SMTPHost:     p.cfg.SMTPHost,
			SMTPPort:     p.cfg.SMTPPort,
			SMTPUsername: name,
			SMTPSender:   p.cfg.Sender,
			Recipients:   []recipient{{Email: req.To}},
			Subject:      req.Subject,
			TextBody:     req.Body,
			HTMLBody:     req.HTMLBody,
		}},
	}, nil); err != nil {
		return fmt.Errorf("pingmailer send: %w", err)
	}
	return nil
}

// validateBaseURL requires an absolute HTTPS URL. Plain HTTP is allowed to a
// loopback host, for local development, and to any host when allowHTTP is set.
func validateBaseURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid baseURL: %w", err)
	}
	if u.Scheme == "" || u.Hostname() == "" {
		return errors.New("baseURL must be an absolute URL")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		ip := net.ParseIP(host)
		if allowHTTP || host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return nil
		}
		return errors.New("baseURL must use HTTPS; set allowInsecureHTTP to send to a plain-HTTP host")
	default:
		return fmt.Errorf("baseURL scheme %q is not http or https", u.Scheme)
	}
}
