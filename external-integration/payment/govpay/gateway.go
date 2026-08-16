// Package govpay holds nsw-srilanka's GovPay+ payment gateway integration that
// plugs into the generic core/payment framework. GovPay+ drives two endpoints:
// presentment (validate), which returns the fields to render to the payer, and
// update (webhook), which submits the completed-payment notification.
package govpay

import (
	"context"
	"encoding/json"
	"fmt"

	corepayment "github.com/OpenNSW/core/payment"
)

// GovPayGateway implements corepayment.PaymentGateway for the GovPay+ aggregator.
type GovPayGateway struct {
	cfg Config
	// resolveIdentity recovers the expected GovPay+ identity for a reference
	// number. Optional: when nil the update (webhook) callback is accepted
	// without an identity check, which is how the gateway behaves in tests and
	// for any deployment that has not wired a resolver.
	resolveIdentity IdentityResolver
}

// NewGovPayGateway satisfies corepayment.Factory: it constructs a fully
// configured GovPayGateway from its raw config.
//
// A gateway built this way cannot check the identity on the update (webhook)
// callback, because that callback carries no transaction to compare against.
// Use NewGovPayGatewayFactory to supply a resolver.
func NewGovPayGateway(cfg json.RawMessage) (corepayment.PaymentGateway, error) {
	return NewGovPayGatewayFactory(nil)(cfg)
}

// NewGovPayGatewayFactory returns a corepayment.Factory that builds gateways
// wired to resolve, by reference number, the GovPay+ identity a fee was
// registered under. Pass nil to disable the check on the update callback.
func NewGovPayGatewayFactory(resolveIdentity IdentityResolver) corepayment.Factory {
	return func(cfg json.RawMessage) (corepayment.PaymentGateway, error) {
		var config Config
		if err := json.Unmarshal(cfg, &config); err != nil {
			return nil, err
		}

		return &GovPayGateway{
			cfg:             config,
			resolveIdentity: resolveIdentity,
		}, nil
	}
}

func (g *GovPayGateway) GetFlowType() corepayment.InteractionType {
	return corepayment.FlowTypeInstruction
}

// ValidateMetadata rejects a fee that did not declare the GovPay+ identity it
// is registered under.
//
// Both callbacks check the posted sub-institution and service ids against what
// the fee declared, so a fee missing either could never be settled — it would
// fail at presentment, by which time the artifact responsible is out of reach.
// Enforcing it here means the misconfiguration surfaces the first time the fee
// is charged, before any reference number or transaction row exists.
//
// Whether the declared ids are the *right* ones is a separate question,
// answered on the callback paths against the values GovPay+ actually posts.
func (g *GovPayGateway) ValidateMetadata(metadata map[string]string) error {
	if identityFromMetadata(metadata).complete() {
		return nil
	}
	return fmt.Errorf("%w: gateway_metadata must declare %s and %s",
		ErrIdentityNotConfigured, MetadataSubInstID, MetadataServiceID)
}

func (g *GovPayGateway) CreateSession(ctx context.Context, req corepayment.SessionRequest) (*corepayment.SessionResponse, error) {
	return &corepayment.SessionResponse{
		Type:         corepayment.FlowTypeInstruction,
		Instructions: "Please use the provided reference number to make a payment via your banking app's GovPay option.",
	}, nil
}

// ExtractReferenceNumber pulls the NSW reference number out of a presentment
// request. Per the GovPay+ contract the reference travels as the single data[]
// item named "refNo".
func (g *GovPayGateway) ExtractReferenceNumber(ctx context.Context, referenceData json.RawMessage) (string, error) {
	req, err := parseGovPayRequest(referenceData)
	if err != nil {
		return "", err
	}
	return validateRefNoOnly(req.Data)
}
