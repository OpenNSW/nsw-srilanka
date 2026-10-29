package config

import (
	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/notification/providers"
)

// NotificationConfig holds the notification section of config.yaml: one block
// per provider, each in that provider's own typed config. The fields are typed,
// so a secret that only looks like a number or a boolean (a password of
// 12345678, a SID code of 0123) stays the string it was written as.
//
// Optional: with no provider set, the server sends no email or SMS, so local
// development needs no gateway credentials.
type NotificationConfig struct {
	Providers NotificationProviders `yaml:"providers"`
}

// NotificationProviders holds the config of each notification provider the
// server runs. Each is optional; a channel whose block is left out sends
// nothing.
type NotificationProviders struct {
	Email *providers.EmailConfig `yaml:"email"`
	SMS   *providers.SMSConfig   `yaml:"sms"`
}

// Validate reports a provider whose config is missing or invalid, by building
// each one set: the constructors hold the providers' rules.
func (c NotificationConfig) Validate() error {
	_, err := c.Build()
	return err
}

// Build returns a provider for each block set, ready to hand to
// notification.NewManager. It returns none when no block is set.
func (c NotificationConfig) Build() ([]notification.Provider, error) {
	var built []notification.Provider
	if c.Providers.Email != nil {
		p, err := providers.NewEmailProvider(*c.Providers.Email)
		if err != nil {
			return nil, err
		}
		built = append(built, p)
	}
	if c.Providers.SMS != nil {
		p, err := providers.NewSMSProvider(*c.Providers.SMS)
		if err != nil {
			return nil, err
		}
		built = append(built, p)
	}
	return built, nil
}
