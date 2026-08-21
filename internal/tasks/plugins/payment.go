package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/OpenNSW/core/payment"
	"github.com/shopspring/decimal"
)

// PaymentPlugin implements a custom generic_payment plugin for taskflow.
// It initiates a checkout session with the payment service and transitions
// the task record state to PENDING_PAYMENT.
type PaymentPlugin struct {
	paymentService payment.PaymentService
}

// NewPaymentPlugin creates a new PaymentPlugin.
func NewPaymentPlugin(paymentService payment.PaymentService) *PaymentPlugin {
	return &PaymentPlugin{
		paymentService: paymentService,
	}
}

type paymentConfig struct {
	TaskCode    string          `json:"task_code"`
	ServiceName string          `json:"service_name"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`

	// Per-fee values the selected gateway needs, passed through opaquely and
	// stored on the transaction so the gateway can recover them at callback
	// time — by then the artifact that declared them is out of reach.
	//
	// This plugin attaches no meaning to the keys; each gateway defines its
	// own, and each gateway enforces its own requirements via
	// corepayment.PaymentGateway.ValidateMetadata — which the payment service
	// calls before it persists anything, so a fee that omitted a required key
	// fails on this Execute rather than at callback time.
	GatewayMetadata map[string]string `json:"gateway_metadata"`
}

// reservedMetadataKeys are written by this plugin and may not be overridden by
// an artifact's gateway_metadata, which would otherwise let a fee rewrite the
// task it belongs to.
var reservedMetadataKeys = map[string]struct{}{
	"task_id":   {},
	"task_code": {},
	"method_id": {},
}

func (p *PaymentPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg paymentConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("payment: failed to parse generic_payment config: %w", err)
	}

	if cfg.Amount.IsZero() {
		return fmt.Errorf("payment: plugin_properties.amount is required and must be non-zero")
	}
	if cfg.Currency == "" {
		return fmt.Errorf("payment: plugin_properties.currency is required")
	}

	// 1. Determine selected payment gateway
	selectedMethod, _ := ctx.Inputs["selected_method"].(string)
	if selectedMethod == "" {
		// Fallback for a fee whose form does not offer a choice. It has to name a
		// method the registry actually carries, or checkout fails on a method that
		// does not exist; kept as a literal so this package stays unaware of any
		// particular gateway's package.
		selectedMethod = "govpay"
	}

	// 2. Transition task state to PENDING_PAYMENT
	ctx.Record.State = "PENDING_PAYMENT"

	amount := cfg.Amount
	currency := cfg.Currency

	slog.Info("task payment: initiating checkout session",
		"taskId", ctx.Record.TaskID, "taskCode", cfg.TaskCode, "amount", amount, "method", selectedMethod)

	// 3. Create the checkout session via core/payment. The selected gateway is
	// passed as GatewayID; the service generates the TNSW- reference and (for
	// instruction-flow gateways) returns the instructions to display. An unknown
	// gateway surfaces here as an error, as does a fee whose gateway_metadata
	// omits something the selected gateway requires — the service asks the
	// gateway to vet the metadata before it persists anything, so the task_code
	// wrapped in below still names the artifact that has to be fixed.
	resp, err := p.paymentService.CreateCheckoutSession(ctx.Context, payment.CreateCheckoutRequest{
		GatewayID: selectedMethod,
		Amount:    amount,
		Currency:  currency,
		ExpiresAt: time.Now().Add(24 * time.Hour), // Aligned with typical TTL
		Metadata:  buildPaymentMetadata(ctx.Record.TaskID, cfg, selectedMethod),
	})
	if err != nil {
		return fmt.Errorf("payment: failed to create checkout session (task_code %q): %w", cfg.TaskCode, err)
	}

	slog.Info("task payment: checkout session registered",
		"taskId", ctx.Record.TaskID, "sessionId", resp.SessionID, "referenceNumber", resp.ReferenceNumber, "method", selectedMethod)

	// 6. Populate payment info under the active output namespace
	if ctx.OutputNamespace != "" {
		if ctx.Record.Data == nil {
			ctx.Record.Data = make(map[string]any)
		}

		serviceName := cfg.ServiceName
		if serviceName == "" {
			serviceName = "Payment"
		}

		pData := map[string]any{
			"session_id":       resp.SessionID,
			"reference_number": resp.ReferenceNumber,
			"amount":           amount.String(),
			"currency":         currency,
			"selected_method":  selectedMethod,
			"checkout_url":     resp.CheckoutURL,
			"instructions":     resp.Instructions,
			"flow_type":        string(resp.Type),
			"service_name":     serviceName,
			"service_type":     cfg.TaskCode,
		}

		ctx.Record.Data[ctx.OutputNamespace] = pData
	}

	// Suspend the workflow until LankaPay/webhook callback arrives
	return ErrSuspended
}

// buildPaymentMetadata assembles the gateway metadata persisted with the
// transaction: this plugin's own bookkeeping plus whatever the artifact
// declared for the gateway, passed through untouched apart from trimming.
//
// The artifact's values are applied first so the reserved keys below always
// win; a fee cannot rewrite the task it belongs to.
func buildPaymentMetadata(taskID string, cfg paymentConfig, selectedMethod string) map[string]string {
	metadata := make(map[string]string, len(cfg.GatewayMetadata)+len(reservedMetadataKeys))

	for key, value := range cfg.GatewayMetadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if _, reserved := reservedMetadataKeys[key]; reserved {
			slog.Warn("task payment: ignoring reserved gateway_metadata key",
				"taskId", taskID, "taskCode", cfg.TaskCode, "key", key)
			continue
		}
		metadata[key] = value
	}

	metadata["task_id"] = taskID
	metadata["task_code"] = cfg.TaskCode
	metadata["method_id"] = selectedMethod
	return metadata
}
