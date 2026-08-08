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

	// requiredFeeMetadata lists, per payment method id, the gateway_metadata
	// keys a fee must declare. Supplied by the caller that wires the gateways,
	// so this plugin needs no knowledge of any of them.
	requiredFeeMetadata map[string][]string
}

// NewPaymentPlugin creates a new PaymentPlugin.
//
// requiredFeeMetadata declares the gateway_metadata keys each payment method
// cannot operate without; methods absent from the map require none. Keeping it
// a parameter is what lets a new gateway be added without touching this file.
func NewPaymentPlugin(paymentService payment.PaymentService, requiredFeeMetadata map[string][]string) *PaymentPlugin {
	return &PaymentPlugin{
		paymentService:      paymentService,
		requiredFeeMetadata: requiredFeeMetadata,
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
	// own. GovPay+, for instance, expects govpay_subinst_id and
	// govpay_service_id, the ids identifying the service a fee is registered
	// under. Which keys a gateway requires is declared where the gateways are
	// wired, not here (see PaymentPlugin.requiredFeeMetadata).
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
		selectedMethod = "lankapay" // Default fallback
	}

	if err := p.requireFeeMetadata(selectedMethod, cfg); err != nil {
		return err
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
	// gateway surfaces here as an error.
	resp, err := p.paymentService.CreateCheckoutSession(ctx.Context, payment.CreateCheckoutRequest{
		GatewayID: selectedMethod,
		Amount:    amount,
		Currency:  currency,
		ExpiresAt: time.Now().Add(24 * time.Hour), // Aligned with typical TTL
		Metadata:  buildPaymentMetadata(ctx.Record.TaskID, cfg, selectedMethod),
	})
	if err != nil {
		return fmt.Errorf("payment: failed to create checkout session: %w", err)
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

// requireFeeMetadata enforces the gateway_metadata keys the selected gateway
// cannot operate without.
//
// A gateway that checks a value on its callbacks can never settle a fee that
// omitted it, so the omission is caught here — while the artifact responsible
// is still named in the error — rather than at callback time, when it is long
// out of reach. Which keys matter is supplied at construction, so this plugin
// stays ignorant of any particular gateway.
func (p *PaymentPlugin) requireFeeMetadata(selectedMethod string, cfg paymentConfig) error {
	missing := make([]string, 0, len(p.requiredFeeMetadata[selectedMethod]))
	for _, key := range p.requiredFeeMetadata[selectedMethod] {
		if strings.TrimSpace(cfg.GatewayMetadata[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"payment: plugin_properties.gateway_metadata is missing %s, required for %s payments (task_code %q)",
			strings.Join(missing, " and "), selectedMethod, cfg.TaskCode)
	}
	return nil
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
