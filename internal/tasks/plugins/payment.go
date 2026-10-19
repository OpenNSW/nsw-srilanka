package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/OpenNSW/core/payment"
	coreplugins "github.com/OpenNSW/core/taskflow/plugins"
	nswpayment "github.com/OpenNSW/nsw-srilanka/internal/payment"
	"github.com/shopspring/decimal"
)

// PaymentPlugin implements a custom generic_payment plugin for taskflow.
// It initiates a checkout session with the payment service and transitions
// the task record state to PENDING_PAYMENT.
type PaymentPlugin struct {
	paymentService payment.PaymentService
	fees           nswpayment.Fees
}

// NewPaymentPlugin creates a new PaymentPlugin that charges the fees in fees
// (the "fees" list of payment_methods.json).
func NewPaymentPlugin(paymentService payment.PaymentService, fees nswpayment.Fees) *PaymentPlugin {
	return &PaymentPlugin{
		paymentService: paymentService,
		fees:           fees,
	}
}

// paymentProperties is a PAYMENT step's plugin_properties: the task code of
// the fee it charges. Everything else about the fee is in payment_methods.json,
// so any other property an artifact still carries is ignored.
type paymentProperties struct {
	TaskCode string `json:"task_code"`
}

// reservedMetadataKeys are written by this plugin and may not be overridden by
// a fee's gateway_metadata, which would otherwise let a fee rewrite the task it
// belongs to.
var reservedMetadataKeys = map[string]struct{}{
	"task_id":   {},
	"task_code": {},
	"method_id": {},
}

// isReservedMetadataKey reports whether key is one this plugin writes: its own
// bookkeeping, or the fee's reference format.
func isReservedMetadataKey(key string) bool {
	_, reserved := reservedMetadataKeys[key]
	return reserved || nswpayment.IsReferenceMetadataKey(key)
}

func (p *PaymentPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var props paymentProperties
	if err := json.Unmarshal(configRaw, &props); err != nil {
		return fmt.Errorf("payment: failed to parse generic_payment config: %w", err)
	}
	if strings.TrimSpace(props.TaskCode) == "" {
		return fmt.Errorf("payment: plugin_properties.task_code is required")
	}
	cfg, ok := p.fees.Lookup(props.TaskCode)
	if !ok {
		return fmt.Errorf("payment: no fee for task_code %q in payment_methods.json fees", props.TaskCode)
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

	// 2. Resolve the amount to charge. If a workflow input is available, it takes
	// precedence over the fee's configured amount. This is the last
	// checkpoint before it becomes an actual charge so the logical validation is done to
	// make sure the payment amount is acceptable.
	amount, err := resolvePaymentAmount(ctx.Inputs["amount"], cfg.Amount)
	if err != nil {
		return fmt.Errorf("payment: %w (task_code %q)", err, cfg.TaskCode)
	}
	currency := cfg.Currency

	referenceMetadata, err := paymentReferenceMetadata(ctx.Inputs, cfg.Reference)
	if err != nil {
		return fmt.Errorf("payment: reference (task_code %q): %w", cfg.TaskCode, err)
	}

	// 3. Transition task state to PENDING_PAYMENT
	ctx.Record.State = "PENDING_PAYMENT"

	slog.Info("task payment: initiating checkout session",
		"taskId", ctx.Record.TaskID, "taskCode", cfg.TaskCode, "amount", amount, "method", selectedMethod)

	// 4. Create the checkout session via core/payment. The selected gateway is
	// passed as GatewayID; the service takes the reference from
	// nswpayment.RefGenerator, in the fee's format, and (for instruction-flow
	// gateways) returns the instructions to display. An unknown
	// gateway surfaces here as an error, as does a fee whose gateway_metadata
	// omits something the selected gateway requires — the service asks the
	// gateway to vet the metadata before it persists anything, so the task_code
	// wrapped in below still names the fee that has to be fixed.
	// The settlement completes exactly this step: the payment service hands the
	// token back to the task manager when the webhook arrives.
	callbackToken, err := coreplugins.CallbackToken(ctx.Record)
	if err != nil {
		return fmt.Errorf("payment: %w", err)
	}
	resp, err := p.paymentService.CreateCheckoutSession(ctx.Context, payment.CreateCheckoutRequest{
		GatewayID:     selectedMethod,
		Amount:        amount,
		Currency:      currency,
		ExpiresAt:     time.Now().Add(24 * time.Hour), // Aligned with typical TTL
		Metadata:      buildPaymentMetadata(ctx.Record.TaskID, cfg, selectedMethod, referenceMetadata),
		CallbackToken: callbackToken,
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
// transaction: this plugin's own bookkeeping, the fee's reference format
// (referenceMetadata, from paymentReferenceMetadata), plus whatever the
// fee declares for the gateway, passed through untouched apart from trimming.
//
// The fee's values are applied first so the reserved keys below always
// win; a fee cannot rewrite the task it belongs to.
func buildPaymentMetadata(taskID string, cfg nswpayment.Fee, selectedMethod string, referenceMetadata map[string]string) map[string]string {
	metadata := make(map[string]string, len(cfg.GatewayMetadata)+len(reservedMetadataKeys))

	for key, value := range cfg.GatewayMetadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if isReservedMetadataKey(key) {
			slog.Warn("task payment: ignoring reserved gateway_metadata key",
				"taskId", taskID, "taskCode", cfg.TaskCode, "key", key)
			continue
		}
		metadata[key] = value
	}

	metadata["task_id"] = taskID
	metadata["task_code"] = cfg.TaskCode
	metadata["method_id"] = selectedMethod
	for key, value := range referenceMetadata {
		metadata[key] = value
	}
	return metadata
}

// paymentReferenceMetadata resolves a fee's reference format into the checkout
// metadata nswpayment.RefGenerator reads: its issuer and id_type, its params read
// through their pointers into inputs, and its fixed values. A fee without a
// format adds no keys, so its checkout takes the default format.
func paymentReferenceMetadata(inputs map[string]any, cfg *nswpayment.FeeReference) (map[string]string, error) {
	if cfg == nil {
		return nil, nil
	}
	if strings.TrimSpace(cfg.Issuer) == "" || strings.TrimSpace(cfg.IDType) == "" {
		return nil, errors.New("reference needs an issuer and an id_type")
	}
	pointers := make(map[string]refIDPointer, len(cfg.Params))
	for name, raw := range cfg.Params {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("reference.params has an empty name")
		}
		ptr, err := parseRefIDPointer(raw, false)
		if err != nil {
			return nil, fmt.Errorf("reference.params.%s: %w", name, err)
		}
		pointers[name] = ptr
	}
	for name := range cfg.Values {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("reference.values has an empty name")
		}
		if _, both := pointers[name]; both {
			return nil, fmt.Errorf("reference names %q in both params and values", name)
		}
	}
	params, err := readRefIDParams(inputs, nil, pointers)
	if err != nil {
		return nil, err
	}
	for name, value := range cfg.Values {
		params[name] = value
	}
	return nswpayment.ReferenceMetadata(cfg.Issuer, cfg.IDType, params), nil
}

// Decides the amount to charge for a task. If the workflow input "amount" is
// supplied, it takes precedence over the fee's configured amount. If no
// input is supplied, the configured amount will be taken. Both are validated to be
// valid as a payment amount.
func resolvePaymentAmount(input any, configured decimal.Decimal) (decimal.Decimal, error) {
	if input == nil {
		if !configured.IsPositive() {
			return decimal.Decimal{}, errors.New(`the fee's amount is required and must be positive when no "amount" input is supplied`)
		}
		return configured, nil
	}

	amount, err := decimalFromAny(input)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf(`input "amount" is invalid: %w`, err)
	}
	if !amount.IsPositive() {
		return decimal.Decimal{}, fmt.Errorf(`input "amount" must be positive, got %s`, amount.String())
	}
	return amount, nil
}

// decimalFromAny converts a workflow input value into a decimal. Workflow
// variables cross a JSON boundary at least once (from form submission -> stored
// task data -> input_mapping copy), so a number arrives here as float64;
// string and json.Number are accepted defensively for callers that pass
// amounts as quoted values.
func decimalFromAny(v any) (decimal.Decimal, error) {
	switch x := v.(type) {
	case float64:
		return decimal.NewFromFloat(x), nil
	case string:
		return decimal.NewFromString(strings.TrimSpace(x))
	case json.Number:
		return decimal.NewFromString(x.String())
	default:
		return decimal.Decimal{}, fmt.Errorf("unsupported type %T", v)
	}
}
