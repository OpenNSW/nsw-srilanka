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

// GovPayGateway implements corepayment.PaymentGateway for the GovPay+ aggregator.
type GovPayGateway struct {
	cfg Config
}

// NewGovPayGateway satisfies corepayment.Factory: it constructs a fully
// configured GovPayGateway from its raw config.
func NewGovPayGateway(cfg json.RawMessage) (corepayment.PaymentGateway, error) {
	var config Config
	if err := json.Unmarshal(cfg, &config); err != nil {
		return nil, err
	}

	return &GovPayGateway{
		cfg: config,
	}, nil
}

func (g *GovPayGateway) GetFlowType() corepayment.InteractionType {
	return corepayment.FlowTypeInstruction
}

func (g *GovPayGateway) CreateSession(ctx context.Context, req corepayment.SessionRequest) (*corepayment.SessionResponse, error) {
	return &corepayment.SessionResponse{
		Type:         corepayment.FlowTypeInstruction,
		Instructions: "Please pay using your bank application. Enter the provided reference number in the bill payment section of your app.",
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
