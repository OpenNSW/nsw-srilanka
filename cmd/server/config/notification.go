package config

import (
	notification "github.com/OpenNSW/core/notifications"
	"github.com/OpenNSW/core/notifications/providers"
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
	Email providers.EmailConfig `yaml:"email"`
	SMS   providers.SMSConfig   `yaml:"sms"`
}

// Validate reports a provider whose config is missing or invalid, by building
// each provider without starting it.
func (c NotificationConfig) Validate() error {
	_, err := c.NewProviders()
	return err
}

// NewProviders builds the email and SMS providers from their config, ready to
// hand to notification.NewManager.
func (c NotificationConfig) NewProviders() ([]notification.Provider, error) {
	email, err := providers.NewEmailProvider(c.Providers.Email)
	if err != nil {
		return nil, err
	}
	sms, err := providers.NewSMSProvider(c.Providers.SMS)
	if err != nil {
		return nil, err
	}
	return []notification.Provider{email, sms}, nil
}
