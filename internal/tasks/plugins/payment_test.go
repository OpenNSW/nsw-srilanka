package plugins

import (
	"context"
	"testing"

	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePaymentAmount(t *testing.T) {
	configured := decimal.NewFromFloat(12500.00)

	tests := []struct {
		name       string
		input      any
		configured decimal.Decimal
		want       string
		wantErr    bool
	}{
		{
			name:       "workflow-supplied float64 overrides configured amount",
			input:      4500.50,
			configured: configured,
			want:       "4500.5",
		},
		{
			// float roundoff check
			name:       "workflow-supplied float64 keeps full precision, not just 2 decimal places",
			input:      4500.567,
			configured: configured,
			want:       "4500.567",
		},
		{
			name:       "workflow-supplied string amount is accepted",
			input:      "4500.50",
			configured: configured,
			want:       "4500.5",
		},
		{
			name:       "no input falls back to configured amount",
			input:      nil,
			configured: configured,
			want:       "12500",
		},
		{
			name:       "no input and no configured amount is an error",
			input:      nil,
			configured: decimal.Decimal{},
			wantErr:    true,
		},
		{
			name:       "zero workflow-supplied amount is rejected even with a valid configured fallback",
			input:      0.0,
			configured: configured,
			wantErr:    true,
		},
		{
			name:       "negative workflow-supplied amount is rejected",
			input:      -100.0,
			configured: configured,
			wantErr:    true,
		},
		{
			name:       "unparseable string amount is rejected",
			input:      "not-a-number",
			configured: configured,
			wantErr:    true,
		},
		{
			name:       "decimal.Decimal is never a valid input type",
			input:      decimal.NewFromFloat(4500.50),
			configured: configured,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePaymentAmount(tt.input, tt.configured)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolvePaymentAmount(%v, %v) = %v, want error", tt.input, tt.configured, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePaymentAmount(%v, %v) unexpected error: %v", tt.input, tt.configured, err)
			}
			if got.String() != tt.want {
				t.Fatalf("resolvePaymentAmount(%v, %v) = %s, want %s", tt.input, tt.configured, got.String(), tt.want)
			}
		})
	}
}

// fakePaymentService records the request it was called with. Embedding the
// interface leaves every method but CreateCheckoutSession unimplemented,
// which is fine as long as Execute never reaches them.
type fakePaymentService struct {
	payment.PaymentService

	lastReq payment.CreateCheckoutRequest
	called  bool
	resp    *payment.CreateCheckoutResponse
	err     error
}

func (f *fakePaymentService) CreateCheckoutSession(_ context.Context, req payment.CreateCheckoutRequest) (*payment.CreateCheckoutResponse, error) {
	f.called = true
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// The task and step IDs are UUIDs, as core mints them: the plugin builds the
// checkout's callback token from them.
const (
	paymentTestTaskID = "6aad0417-9a6d-4407-9509-2e51d8fcae99"
	paymentTestStepID = "ca7ed707-1dba-43ca-94bf-10eddf00df3c"
)

func paymentPluginCtx(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Context: context.Background(),
		Inputs:  inputs,
		Record:  &store.TaskRecord{TaskID: paymentTestTaskID, ActiveStepID: paymentTestStepID, Data: map[string]any{}},
	}
}

const paymentConfigJSON = `{"task_code":"CODE1","currency":"LKR","amount":12500}`

func TestPaymentPlugin_Execute_UsesWorkflowAmountOverConfigured(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := NewPaymentPlugin(svc)

	err := p.Execute(paymentPluginCtx(map[string]any{"amount": 4500.50}), []byte(paymentConfigJSON))

	require.ErrorIs(t, err, ErrSuspended)
	require.True(t, svc.called, "expected the payment service to be called")
	assert.True(t, svc.lastReq.Amount.Equal(decimal.NewFromFloat(4500.50)),
		"expected the per-consignment workflow amount to reach the gateway, got %s (configured amount was 12500)", svc.lastReq.Amount)
}

func TestPaymentPlugin_Execute_FallsBackToConfiguredAmountWhenNoInput(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := NewPaymentPlugin(svc)

	err := p.Execute(paymentPluginCtx(nil), []byte(paymentConfigJSON))

	require.ErrorIs(t, err, ErrSuspended)
	require.True(t, svc.called, "expected the payment service to be called")
	assert.True(t, svc.lastReq.Amount.Equal(decimal.NewFromInt(12500)),
		"expected the configured amount to reach the gateway, got %s", svc.lastReq.Amount)
}

// The checkout carries the token for the step that dispatched it, so the
// settlement completes exactly that step.
func TestPaymentPlugin_Execute_SendsTheStepsCallbackToken(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := NewPaymentPlugin(svc)

	err := p.Execute(paymentPluginCtx(nil), []byte(paymentConfigJSON))

	require.ErrorIs(t, err, ErrSuspended)
	want, err := callbacktoken.Encode(paymentTestTaskID, paymentTestStepID)
	require.NoError(t, err)
	assert.Equal(t, want, svc.lastReq.CallbackToken)
}

func TestPaymentPlugin_Execute_RejectsInvalidAmountWithoutCallingGateway(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := NewPaymentPlugin(svc)

	ctx := paymentPluginCtx(map[string]any{"amount": -5.0})
	err := p.Execute(ctx, []byte(paymentConfigJSON))

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrSuspended)
	assert.False(t, svc.called, "the gateway must never be called with a rejected amount")
	assert.NotEqual(t, "PENDING_PAYMENT", ctx.Record.State,
		"the task must not be marked PENDING_PAYMENT when the amount is rejected")
}
