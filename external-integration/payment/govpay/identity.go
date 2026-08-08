package govpay

import (
	"context"
	"fmt"
	"strings"
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
// The expected pair is declared per fee in the artifact's PAYMENT
// plugin_properties (govpay_subinst_id / govpay_service_id) and recorded on the
// transaction when the fee is registered, so it can be recovered later by
// reference number.
const (
	// MetadataSubInstID keys the expected sub-institution id in the payment
	// transaction's gateway metadata.
	MetadataSubInstID = "govpay_subinst_id"
	// MetadataServiceID keys the expected service id.
	MetadataServiceID = "govpay_service_id"
)

// ExpectedIdentity is the GovPay+ identity a reference number was registered
// under. Both fields are required for a GovPay+ fee: an empty one means the
// artifact failed to declare it, which is a misconfiguration to report rather
// than a reason to let the call through unchecked.
type ExpectedIdentity struct {
	SubInstID string
	ServiceID string
}

// complete reports whether the artifact declared both ids.
func (e ExpectedIdentity) complete() bool {
	return e.SubInstID != "" && e.ServiceID != ""
}

// IdentityResolver recovers the expected identity for a reference number.
// found is false when the reference is unknown to this deployment.
//
// The update (webhook) callback carries no transaction — the core gateway
// contract hands it only the raw body — so the gateway is given this resolver
// at construction to look the reference up for itself. The presentment
// (validate) callback needs no resolver: it already receives the transaction,
// metadata included.
type IdentityResolver func(ctx context.Context, referenceNumber string) (identity ExpectedIdentity, found bool, err error)

// identityFromMetadata reads the expected identity off a transaction's gateway
// metadata.
func identityFromMetadata(metadata map[string]string) ExpectedIdentity {
	return ExpectedIdentity{
		SubInstID: strings.TrimSpace(metadata[MetadataSubInstID]),
		ServiceID: strings.TrimSpace(metadata[MetadataServiceID]),
	}
}

// verify reports whether a request's identity matches what the fee was
// registered under. Both ids must have been declared and both must match.
//
// Matching ignores case and surrounding space: the ids are onboarding-time
// constants that appear in configuration by hand, and GovPay+ environments have
// been seen to differ in the casing of alphabetic ids.
func (e ExpectedIdentity) verify(gotSubInstID, gotServiceID string) error {
	if !e.complete() {
		return fmt.Errorf("%w: govpay_subinst_id and govpay_service_id must both be set in the fee's plugin_properties",
			ErrIdentityNotConfigured)
	}
	if err := matchField("subinstId", e.SubInstID, gotSubInstID); err != nil {
		return err
	}
	return matchField("serviceid", e.ServiceID, gotServiceID)
}

// verifyWebhookIdentity checks an update (webhook) notification against the
// identity its reference number was registered under. The notification is
// rejected unless the fee declared both ids and the caller sent matching ones.
//
// An unknown reference is the one case passed through: the service layer looks
// the reference up straight afterwards and owns that decision, and failing here
// too would report the same thing two different ways.
func (g *GovPayGateway) verifyWebhookIdentity(ctx context.Context, refNo string, req govPayRequest) error {
	if g.resolveIdentity == nil {
		// Reaching a live webhook without a resolver means the gateway was
		// built by the bare constructor instead of NewGovPayGatewayFactory.
		// The identity cannot be checked, so the notification is refused
		// rather than settled unverified.
		return fmt.Errorf("webhook for %s: %w: no identity resolver wired for the govpay gateway",
			refNo, ErrIdentityNotConfigured)
	}

	expected, found, err := g.resolveIdentity(ctx, refNo)
	if err != nil {
		return fmt.Errorf("resolving expected identity for %s: %w", refNo, err)
	}
	if !found {
		return nil
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
