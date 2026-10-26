package config

import (
	"fmt"
	"log/slog"

	notification "github.com/OpenNSW/core/notifications"
	"github.com/OpenNSW/core/notifications/providers"

	"github.com/OpenNSW/nsw-srilanka/external-integration/pingmailer"
)

// NotificationConfig holds the notification section of config.yaml: one block
// per provider, each in that provider's own typed config. The fields are typed,
// so a secret that only looks like a number or a boolean (a password of
// 12345678, a SID code of 0123) stays the string it was written as.
type NotificationConfig struct {
	Providers NotificationProviders `yaml:"providers"`
}

// NotificationProviders holds the config of each notification provider the
// server runs. Every provider is required.
type NotificationProviders struct {
	Email EmailProviderConfig `yaml:"email"`
	SMS   providers.SMSConfig `yaml:"sms"`
}

// Email provider types, named by notification.providers.email.type.
const (
	// EmailProviderHTTP posts {to, subject, body, htmlBody} to <baseURL>/send
	// with a fixed bearer token. The default.
	EmailProviderHTTP = "http"
	// EmailProviderPingMailer sends through PingMailer, with a token from an
	// OAuth2 client.
	EmailProviderPingMailer = "pingmailer"
)

// EmailProviderConfig selects the email provider. With no type, or type http,
// it is core's provider, configured by baseURL and token. With type pingmailer,
// it is configured by the pingmailer block alone.
type EmailProviderConfig struct {
	Type                  string `yaml:"type"`
	providers.EmailConfig `yaml:",inline"`
	PingMailer            pingmailer.Config `yaml:"pingmailer"`
}

// Build returns the email provider the config selects, ready to send.
func (c EmailProviderConfig) Build() (notification.Provider, error) {
	switch c.Type {
	case "", EmailProviderHTTP:
		return providers.NewEmailProvider(c.EmailConfig)
	case EmailProviderPingMailer:
		if c.BaseURL != "" || c.Token != "" {
			return nil, fmt.Errorf("email: baseURL and token are not used with type %s; set them under pingmailer", EmailProviderPingMailer)
		}
		if a := c.PingMailer.Auth.OAuth2; a != nil && a.InsecureSkipTLSVerify && !isDevEnvironment() {
			return nil, fmt.Errorf("email: pingmailer.auth.oauth2.insecureSkipTLSVerify: APP_ENV is not \"development\" (unset or any other value is treated as production); refusing to start")
		}
		return pingmailer.New(c.PingMailer)
	default:
		return nil, fmt.Errorf("email: unknown type %q (want %s or %s)", c.Type, EmailProviderHTTP, EmailProviderPingMailer)
	}
}

// Validate reports a provider whose config is missing or invalid, by building
// each provider and discarding it: the constructors hold the providers' rules.
func (c NotificationConfig) Validate() error {
	if _, err := c.Providers.Email.Build(); err != nil {
		return err
	}
	if _, err := providers.NewSMSProvider(c.Providers.SMS); err != nil {
		return err
	}
	if p := c.Providers.Email; p.Type == EmailProviderPingMailer && p.PingMailer.AllowInsecureHTTP {
		slog.Warn("notification.providers.email.pingmailer.allowInsecureHTTP is set: the email access token is sent over plain HTTP",
			"baseURL", p.PingMailer.BaseURL)
	}
	return nil
}
