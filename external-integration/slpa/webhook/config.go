package webhook

import (
	"errors"
	"strings"
)

// Config is what a deployment owes this webhook.
//
// It lives here rather than in the server's configuration package so that what
// the integration needs, and what counts as usable, are stated where the
// integration is: the server reads the environment and hands the value over,
// as it does for every other component that owns its own configuration.
type Config struct {
	// Secret is shared with SLPA out of band. Their Cargo Management System
	// signs each callback with it, and that signature is the whole of the
	// route's authentication — there is no token in front of it.
	Secret string
}

// ErrNoSecret reports a deployment that has not been given the shared secret.
var ErrNoSecret = errors.New("slpa webhook: signing secret is required (set SLPA_WEBHOOK_SECRET)")

// Validate reports whether the webhook can be mounted.
//
// A deployment without the secret cannot hear SLPA's decisions, so it would
// accept service orders whose answer never arrives: the callback is refused,
// their retries stop, and the consignment waits on an approval that has already
// happened. Better to say so before the service starts than to leave it to be
// found by a trader whose consignment stopped.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Secret) == "" {
		return ErrNoSecret
	}
	return nil
}
