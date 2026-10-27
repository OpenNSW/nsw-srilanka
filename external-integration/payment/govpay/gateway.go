// Package govpay holds nsw-srilanka's GovPay+ payment gateway integration that
// plugs into the generic core/payment framework. GovPay+ drives two endpoints:
// presentment (validate), which returns the fields to render to the payer, and
// update (webhook), which submits the completed-payment notification.
package govpay

import (
	"context"
	"encoding/json"

	corepayment "github.com/OpenNSW/core/payment"
)

// GovPayGateway implements corepayment.Gateway for the GovPay+ aggregator.
type GovPayGateway struct {
	cfg Config
	// decryptor holds this GO's RSA private key. GovPay+ encrypts every call
	// (spec §3), so it is required in any deployment GovPay+ actually calls.
	// It is nil when no key is configured; the callback paths then fail with
	// ErrEncryptionNotConfigured, which names the cause instead of panicking.
	decryptor *Decryptor
}

// NewGovPayGateway satisfies corepayment.Factory: it constructs a fully
// configured GovPayGateway from its method's config block in config.yaml.
func NewGovPayGateway(decode func(v any) error) (corepayment.Gateway, error) {
	var config Config
	if err := decode(&config); err != nil {
		return nil, err
	}

	// A key that is configured but unreadable is a hard failure: starting
	// up and failing every call later would be strictly worse than
	// refusing to build the gateway now.
	decryptor, err := loadDecryptor(config.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &GovPayGateway{
		cfg:       config,
		decryptor: decryptor,
	}, nil
}

func (g *GovPayGateway) GetFlowType() corepayment.InteractionType {
	return corepayment.FlowTypeInstruction
}

// ValidatePayeeMapping rejects a payee mapping that does not declare the
// GovPay+ identity the payee's fees are registered under.
//
// Both callbacks check the posted sub-institution and service ids against the
// mapping, so a payee missing either could never be settled. core/payment calls
// this for every mapping when it starts, so the misconfiguration stops startup
// instead of surfacing at presentment.
//
// Whether the declared ids are the *right* ones is a separate question,
// answered on the callback paths against the values GovPay+ actually posts.
func (g *GovPayGateway) ValidatePayeeMapping(mapping json.RawMessage) error {
	_, err := parseIdentity(mapping)
	return err
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
	req, _, err := g.decryptRequest(ctx, referenceData)
	if err != nil {
		return "", err
	}
	return validateRefNoOnly(req.Data)
}
