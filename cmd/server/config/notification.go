package config

import "github.com/OpenNSW/core/notifications/providers"

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
// each provider and discarding it: the constructors hold the providers' rules.
func (c NotificationConfig) Validate() error {
	if _, err := providers.NewEmailProvider(c.Providers.Email); err != nil {
		return err
	}
	if _, err := providers.NewSMSProvider(c.Providers.SMS); err != nil {
		return err
	}
	return nil
}
