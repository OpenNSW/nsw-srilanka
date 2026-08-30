// Package integrations gathers what the deployment owes the systems NSW talks
// to directly.
//
// Most of an integration's configuration is not here: the outbound calls are
// described in the services registry (see configs/services*.json), which the
// remote manager reads on its own. What is left is everything else a partner
// system needs from a deployment — the secret SLPA signs its callbacks with,
// today — and it lives beside the integrations rather than in the server's
// configuration package, so an integration states its own requirements and the
// server only says where the values come from.
//
// A new integration adds a field here and validates itself; nothing in cmd/
// needs to learn what it requires.
package integrations

import (
	"fmt"

	slpawebhook "github.com/OpenNSW/nsw-srilanka/external-integration/slpa/webhook"
)

// Config is the configuration of every external integration, as one value.
//
// The fields are the plain values a deployment supplies, so the server can fill
// them from the environment without importing an integration or knowing what
// shape it wants them in. The accessors below hand each integration its own
// configuration, and the integration says what makes it usable.
type Config struct {
	// SLPAWebhookSecret is shared with SLPA out of band and authenticates the
	// callbacks their Cargo Management System makes to report a service order's
	// progress.
	SLPAWebhookSecret string
}

// SLPAWebhook is the configuration of the SLPA service order webhook.
func (c Config) SLPAWebhook() slpawebhook.Config {
	return slpawebhook.Config{Secret: c.SLPAWebhookSecret}
}

// Validate reports the first integration whose configuration a deployment
// cannot run with.
//
// Each integration answers for itself, so what counts as usable is stated where
// the integration is and this only names which one refused.
func (c Config) Validate() error {
	if err := c.SLPAWebhook().Validate(); err != nil {
		return fmt.Errorf("slpa webhook: %w", err)
	}
	return nil
}
