package govpay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corepayment "github.com/OpenNSW/core/payment"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// GovPay+ posts a sub-institution and a service id on both the presentment
// (validate) and update (webhook) calls. Both are assigned at onboarding and
// identify which service the caller believes it is paying for. Nothing about
// them is secret, so on their own they are not authentication — the endpoints
// sit behind a scoped JWT for that. What they catch is a correctly
// authenticated call aimed at the wrong service: a fee registered under one
// GovPay+ service being settled through another, which would otherwise mark the
// transaction paid on the strength of a mismatched receipt.
//
// The expected pair is this method's mapping for the payee a fee is charged to:
// the govpay entry under "payees" in payment_methods.json, which core/payment
// resolves from the transaction's org_id and service_id.

// payeeMapping is the govpay entry a payee resolves to under "payees" in
// payment_methods.json.
type payeeMapping struct {
	SubInstID string `json:"subinst_id"`
	ServiceID string `json:"service_id"`
}

// ExpectedIdentity is the GovPay+ identity a reference number was registered
// under. Both fields are required for a GovPay+ fee: an empty one means the
// payee's mapping is incomplete, which is a misconfiguration to report rather
// than a reason to let the call through unchecked.
type ExpectedIdentity struct {
	SubInstID string
	ServiceID string
}

// complete reports whether the mapping declared both ids.
func (e ExpectedIdentity) complete() bool {
	return e.SubInstID != "" && e.ServiceID != ""
}

// identityFromMapping reads the expected identity off a payee's govpay mapping.
// The single place a mapping is turned into an identity, so startup validation
// and both callbacks cannot drift apart on how they read it. An absent mapping
// reads as an empty identity.
func identityFromMapping(mapping json.RawMessage) (ExpectedIdentity, error) {
	var m payeeMapping
	if len(mapping) > 0 {
		if err := json.Unmarshal(mapping, &m); err != nil {
			return ExpectedIdentity{}, fmt.Errorf("%w: govpay payee mapping: %v", ErrIdentityNotConfigured, err)
		}
	}
	return ExpectedIdentity{
		SubInstID: strings.TrimSpace(m.SubInstID),
		ServiceID: strings.TrimSpace(m.ServiceID),
	}, nil
}

// verify reports whether a request's identity matches what the fee was
// registered under. Both ids must have been declared and both must match.
//
// Matching ignores case and surrounding space: the ids are onboarding-time
// constants that appear in configuration by hand, and GovPay+ environments have
// been seen to differ in the casing of alphabetic ids.
func (e ExpectedIdentity) verify(gotSubInstID, gotServiceID string) error {
	if !e.complete() {
		return fmt.Errorf("%w: subinst_id and service_id must both be set in the payee's govpay mapping",
			ErrIdentityNotConfigured)
	}
	if err := matchField("subinstId", e.SubInstID, gotSubInstID); err != nil {
		return err
	}
	return matchField("serviceid", e.ServiceID, gotServiceID)
}

// verifyWebhookIdentity checks an update (webhook) notification against the
// identity its reference number was registered under, which payees resolves
// from the transaction's payee. The notification is rejected unless the
// mapping declares both ids and the caller sent matching ones.
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
	expected, err := identityFromMapping(mapping)
	if err != nil {
		return fmt.Errorf("webhook for %s: %w", refNo, err)
	}
	if err := expected.verify(req.SubInstID, req.ServiceID); err != nil {
		return fmt.Errorf("webhook for %s: %w", refNo, err)
	}
	return nil
}

func matchField(field, expected, got string) error {
	if expected == "" {
		return nil // not configured for this fee — nothing to check
	}
	if !strings.EqualFold(strings.TrimSpace(got), expected) {
		return fmt.Errorf("%w: %s %q does not match the %q this reference is registered under",
			ErrIdentityMismatch, field, got, expected)
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
