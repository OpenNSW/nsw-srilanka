package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenNSW/nsw/backend/internal/payments"
	"github.com/shopspring/decimal"
)

// PaymentPlugin implements a custom generic_payment plugin for taskv2.
// It initiates a checkout session with the payment service and transitions
// the task record state to PENDING_PAYMENT.
type PaymentPlugin struct {
	paymentService payments.PaymentService
}

// NewPaymentPlugin creates a new PaymentPlugin.
func NewPaymentPlugin(paymentService payments.PaymentService) *PaymentPlugin {
	return &PaymentPlugin{
		paymentService: paymentService,
	}
}

type paymentConfig struct {
	TaskCode    string          `json:"task_code"`
	ServiceName string          `json:"service_name"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
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

	_, err := p.paymentService.GetPaymentMethod(selectedMethod)
	if err != nil {
		return fmt.Errorf("payment: failed to get payment method %q: %w", selectedMethod, err)
	}

	// 2. Transition task state to PENDING_PAYMENT
	ctx.Record.State = "PENDING_PAYMENT"

	amount := cfg.Amount
	currency := cfg.Currency

	slog.Info("taskv2 payment: initiating checkout session",
		"taskId", ctx.Record.TaskID, "taskCode", cfg.TaskCode, "amount", amount, "method", selectedMethod)

	// 5. Create checkout session in payments.PaymentService to register transaction
	resp, err := p.paymentService.CreateCheckoutSession(ctx.Context, payments.CreateCheckoutRequest{
		ReferenceNumber: "", // Empty string instructs the service to generate a unique TNSW- reference
		Amount:          amount,
		Currency:        currency,
		ExpiresAt:       time.Now().Add(24 * time.Hour), // Aligned with typical TTL
		Metadata: map[string]string{
			"task_id":   ctx.Record.TaskID,
			"task_code": cfg.TaskCode,
			"method_id": selectedMethod,
		},
	})
	if err != nil {
		return fmt.Errorf("payment: failed to create checkout session: %w", err)
	}

	slog.Info("taskv2 payment: checkout session registered",
		"taskId", ctx.Record.TaskID, "sessionId", resp.SessionID, "referenceNumber", resp.ReferenceNumber, "method", selectedMethod)

	// 6. Populate payment info under the active output namespace
	if ctx.Record.ActiveOutputNamespace != "" {
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
			"service_name":     serviceName,
			"service_type":     cfg.TaskCode,
		}

		ctx.Record.Data[ctx.Record.ActiveOutputNamespace] = pData
	}

	// Suspend the workflow until LankaPay/webhook callback arrives
	return ErrSuspended
}
