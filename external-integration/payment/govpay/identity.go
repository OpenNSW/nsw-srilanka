package govpay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corepayment "github.com/OpenNSW/core/payment"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// identity is the GovPay+ sub-institution and service a payee is mapped to:
// the govpay method's "payees" in config.yaml's payment section, which core/payment
// resolves from a transaction's org_id and service_id.
//
// GovPay+ posts both ids on the presentment (validate) and update (webhook)
// calls, naming the service the caller believes it is paying for. They are not
// secret, so they are not authentication — the endpoints sit behind a scoped
// JWT for that. What they catch is a correctly authenticated call aimed at the
// wrong service: a fee registered under one GovPay+ service being settled
// through another, which would otherwise mark the transaction paid on the
// strength of a mismatched receipt.
type identity struct {
	SubInstID string `json:"subinst_id"`
	ServiceID string `json:"service_id"`
}

// parseIdentity reads a payee's govpay mapping. Both ids are required: a
// mapping missing either is a misconfiguration, reported as
// ErrIdentityNotConfigured rather than let through unchecked.
func parseIdentity(mapping json.RawMessage) (identity, error) {
	var id identity
	if len(mapping) > 0 {
		if err := json.Unmarshal(mapping, &id); err != nil {
			return identity{}, fmt.Errorf("%w: govpay payee mapping: %v", ErrIdentityNotConfigured, err)
		}
	}
	id.SubInstID = strings.TrimSpace(id.SubInstID)
	id.ServiceID = strings.TrimSpace(id.ServiceID)
	if id.SubInstID == "" || id.ServiceID == "" {
		return identity{}, fmt.Errorf("%w: a govpay payee mapping must declare subinst_id and service_id",
			ErrIdentityNotConfigured)
	}
	return id, nil
}

// match reports whether a call's ids are the ones the payee is mapped to.
//
// Matching ignores case and surrounding space: the ids are onboarding-time
// constants that appear in configuration by hand, and GovPay+ environments have
// been seen to differ in the casing of alphabetic ids.
func (id identity) match(gotSubInstID, gotServiceID string) error {
	if !strings.EqualFold(strings.TrimSpace(gotSubInstID), id.SubInstID) {
		return fmt.Errorf("%w: subinstId %q does not match the %q this reference is registered under",
			ErrIdentityMismatch, gotSubInstID, id.SubInstID)
	}
	if !strings.EqualFold(strings.TrimSpace(gotServiceID), id.ServiceID) {
		return fmt.Errorf("%w: serviceid %q does not match the %q this reference is registered under",
			ErrIdentityMismatch, gotServiceID, id.ServiceID)
	}
	return nil
}

// verifyWebhookIdentity checks an update (webhook) notification against the
// identity its reference number was registered under, which payees resolves
// from the transaction's payee.
//
// An unknown reference is the one case passed through: the service layer looks
// the reference up straight afterwards and owns that decision, and failing here
// too would report the same thing two different ways.
func (g *GovPayGateway) verifyWebhookIdentity(ctx context.Context, refNo string, req govPayRequest, payees corepayment.PayeeLookup) error {
	if payees == nil {
		// core/payment always passes a lookup; reaching here without one means
		// the gateway was called outside the payment service. The identity
		// cannot be checked, so the notification is refused rather than
		// settled unverified.
		return fmt.Errorf("webhook for %s: %w: no payee lookup to check the identity against",
			refNo, ErrIdentityNotConfigured)
	}

	mapping, found, err := payees(ctx, refNo)
	if err != nil {
		return fmt.Errorf("resolving expected identity for %s: %w", refNo, err)
	}
	if !found {
		return nil
	}
	id, err := parseIdentity(mapping)
	if err == nil {
		err = id.match(req.SubInstID, req.ServiceID)
	}
	if err != nil {
		return fmt.Errorf("webhook for %s: %w", refNo, err)
	}
	return nil
}

// VerifyWebhook authenticates an inbound GovPay+ call — presentment or update —
// as genuinely coming from GovPay+, by the OAuth2 client the request was
// authenticated as.
//
// Both callback routes already sit behind the IdP and a payment webhook scope,
// so this is not the first check the request passes. What it adds is which
// gateway the caller is entitled to act for: the scopes are held per client, not
// per gateway, so without this any machine client granted them could settle a
// GovPay+ transaction. Pinning the caller to the configured client is the
// narrowing the scope check cannot express.
//
// The identity carried *inside* the payload is a separate question — which
// sub-institution and service a reference was registered under — and is checked
// by verifyWebhookIdentity once the body has been parsed.
func (g *GovPayGateway) VerifyWebhook(ctx context.Context, _ []byte, _ map[string][]string) error {
	if g.cfg.WebhookClientID == "" {
		// Operational: a deployment that has not named its client cannot judge
		// this caller either way, so this must not claim the caller is invalid.
		return fmt.Errorf("govpay: cannot verify callback: %w", ErrWebhookClientNotConfigured)
	}

	p, ok := authn.FromContext(ctx)
	if !ok || p.Kind != authn.KindClient {
		// The routes are authenticated, so a call arriving with no machine
		// principal is either unauthenticated or a user token — neither is
		// GovPay+.
		return corepayment.NewWebhookVerificationError("govpay: callback carries no authenticated client")
	}

	if p.ClientID != g.cfg.WebhookClientID {
		return corepayment.NewWebhookVerificationError(
			fmt.Sprintf("govpay: callback authenticated as client %q, expected %q",
				p.ClientID, g.cfg.WebhookClientID))
	}
	return nil
}
