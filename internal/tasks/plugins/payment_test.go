package plugins

import (
	"context"
	"fmt"
	"testing"

	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	nswpayment "github.com/OpenNSW/nsw-srilanka/internal/payment"
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

// paymentProps is the plugin_properties of the tests' PAYMENT step: it names
// the fee by task code, and the plugin finds the rest in its fees.
const paymentProps = `{"task_code":"CODE1"}`

// testFee is the fee the tests' step charges.
func testFee() nswpayment.Fee {
	return nswpayment.Fee{TaskCode: "CODE1", ServiceName: "Test Fee", Amount: decimal.NewFromInt(12500), Currency: "LKR"}
}

// feeReference is a CDA-style reference format: a param read from the inputs
// and two fixed values.
func feeReference() *nswpayment.FeeReference {
	return &nswpayment.FeeReference{
		Issuer: "CDA", IDType: "fee_payment_ref",
		Params: map[string]string{"exporterId": "/exporter_id"},
		Values: map[string]string{"mainCategory": "01", "subCategory": "00002"},
	}
}

// paymentPlugin builds the plugin with fee as its only fee.
func paymentPlugin(svc payment.PaymentService, fee nswpayment.Fee) *PaymentPlugin {
	return NewPaymentPlugin(svc, nswpayment.Fees{Fees: []nswpayment.Fee{fee}})
}

func TestPaymentPlugin_Execute_UsesWorkflowAmountOverConfigured(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := paymentPlugin(svc, testFee())

	err := p.Execute(paymentPluginCtx(map[string]any{"amount": 4500.50}), []byte(paymentProps))

	require.ErrorIs(t, err, ErrSuspended)
	require.True(t, svc.called, "expected the payment service to be called")
	assert.True(t, svc.lastReq.Amount.Equal(decimal.NewFromFloat(4500.50)),
		"expected the per-consignment workflow amount to reach the gateway, got %s (configured amount was 12500)", svc.lastReq.Amount)
}

func TestPaymentPlugin_Execute_FallsBackToConfiguredAmountWhenNoInput(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := paymentPlugin(svc, testFee())

	err := p.Execute(paymentPluginCtx(nil), []byte(paymentProps))

	require.ErrorIs(t, err, ErrSuspended)
	require.True(t, svc.called, "expected the payment service to be called")
	assert.True(t, svc.lastReq.Amount.Equal(decimal.NewFromInt(12500)),
		"expected the configured amount to reach the gateway, got %s", svc.lastReq.Amount)
}

// The checkout carries the token for the step that dispatched it, so the
// settlement completes exactly that step.
func TestPaymentPlugin_Execute_SendsTheStepsCallbackToken(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := paymentPlugin(svc, testFee())

	err := p.Execute(paymentPluginCtx(nil), []byte(paymentProps))

	require.ErrorIs(t, err, ErrSuspended)
	want, err := callbacktoken.Encode(paymentTestTaskID, paymentTestStepID)
	require.NoError(t, err)
	assert.Equal(t, want, svc.lastReq.CallbackToken)
}

func TestPaymentPlugin_Execute_RejectsInvalidAmountWithoutCallingGateway(t *testing.T) {
	svc := &fakePaymentService{resp: &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "TNSW-1", Type: payment.FlowTypeRedirect}}
	p := paymentPlugin(svc, testFee())

	ctx := paymentPluginCtx(map[string]any{"amount": -5.0})
	err := p.Execute(ctx, []byte(paymentProps))

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrSuspended)
	assert.False(t, svc.called, "the gateway must never be called with a rejected amount")
	assert.NotEqual(t, "PENDING_PAYMENT", ctx.Record.State,
		"the task must not be marked PENDING_PAYMENT when the amount is rejected")
}

func checkoutResponse() *payment.CreateCheckoutResponse {
	return &payment.CreateCheckoutResponse{SessionID: "session-1", ReferenceNumber: "000201000020001", Type: payment.FlowTypeInstruction}
}

// generatedFrom feeds a checkout's metadata to nswpayment.RefGenerator, as the
// payment service does, and returns the refid call it makes.
func generatedFrom(t *testing.T, req payment.CreateCheckoutRequest) refIDCall {
	t.Helper()
	refIDs := &fakeRefIDs{}
	_, err := nswpayment.NewRefGenerator(refIDs).GenerateReference(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, refIDs.calls, 1)
	return refIDs.calls[0]
}

// A fee's reference format reaches the payment service in the checkout's
// metadata: the generator draws from its issuer and id_type, with a param read
// from the inputs and the fixed values. The reference the service issues is the
// step's reference_number.
func TestPaymentPlugin_Execute_PassesFeeReferenceFormat(t *testing.T) {
	svc := &fakePaymentService{resp: checkoutResponse()}
	ctx := paymentPluginCtx(map[string]any{"exporter_id": "0002"})
	ctx.OutputNamespace = "payment"

	fee := testFee()
	fee.Reference = feeReference()

	err := paymentPlugin(svc, fee).Execute(ctx, []byte(paymentProps))

	require.ErrorIs(t, err, ErrSuspended)
	assert.Equal(t, refIDCall{issuer: "CDA", idType: "fee_payment_ref", params: map[string]string{
		"exporterId": "0002", "mainCategory": "01", "subCategory": "00002",
	}}, generatedFrom(t, svc.lastReq))
	assert.Equal(t, "000201000020001", ctx.Record.Data["payment"].(map[string]any)["reference_number"])
}

// A reference format that expects no params needs neither params nor values:
// the generator draws from the fee's issuer and id_type alone.
func TestPaymentPlugin_Execute_ReferenceFormatWithoutParams(t *testing.T) {
	svc := &fakePaymentService{resp: checkoutResponse()}

	fee := testFee()
	fee.Reference = &nswpayment.FeeReference{Issuer: "ACME", IDType: "invoice_ref"}

	err := paymentPlugin(svc, fee).Execute(paymentPluginCtx(nil), []byte(paymentProps))

	require.ErrorIs(t, err, ErrSuspended)
	assert.Equal(t, refIDCall{issuer: "ACME", idType: "invoice_ref", params: map[string]string{}}, generatedFrom(t, svc.lastReq))
}

// A fee without a reference format takes the default format.
func TestPaymentPlugin_Execute_DefaultReferenceFormat(t *testing.T) {
	svc := &fakePaymentService{resp: checkoutResponse()}

	err := paymentPlugin(svc, testFee()).Execute(paymentPluginCtx(nil), []byte(paymentProps))

	require.ErrorIs(t, err, ErrSuspended)
	call := generatedFrom(t, svc.lastReq)
	assert.Equal(t, nswpayment.DefaultReferenceIssuer, call.issuer)
	assert.Equal(t, nswpayment.DefaultReferenceIDType, call.idType)
	assert.Empty(t, call.params)
}

// gateway_metadata cannot set the reference format: the generator draws from
// the fee's own format, or the default.
func TestPaymentPlugin_Execute_ReferenceKeysAreReserved(t *testing.T) {
	planted := map[string]string{"reference_issuer": "X", "reference_id_type": "y", "reference_param.exporterId": "9999"}
	for name, tc := range map[string]struct {
		reference *nswpayment.FeeReference
		want      refIDCall
	}{
		"fee without a format": {
			want: refIDCall{issuer: nswpayment.DefaultReferenceIssuer, idType: nswpayment.DefaultReferenceIDType, params: map[string]string{}},
		},
		"fee with a format": {
			reference: &nswpayment.FeeReference{Issuer: "CDA", IDType: "fee_payment_ref", Values: map[string]string{"exporterId": "0002"}},
			want:      refIDCall{issuer: "CDA", idType: "fee_payment_ref", params: map[string]string{"exporterId": "0002"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &fakePaymentService{resp: checkoutResponse()}
			fee := testFee()
			fee.GatewayMetadata = planted
			fee.Reference = tc.reference

			err := paymentPlugin(svc, fee).Execute(paymentPluginCtx(nil), []byte(paymentProps))

			require.ErrorIs(t, err, ErrSuspended)
			assert.Equal(t, tc.want, generatedFrom(t, svc.lastReq))
		})
	}
}

func TestPaymentPlugin_Execute_RejectsMalformedReference(t *testing.T) {
	exporterID := map[string]string{"exporterId": "/exporter_id"}
	for name, tc := range map[string]struct {
		reference nswpayment.FeeReference
		inputs    map[string]any
		want      string
	}{
		"no issuer":  {reference: nswpayment.FeeReference{IDType: "fee_payment_ref"}, want: "needs an issuer and an id_type"},
		"no id_type": {reference: nswpayment.FeeReference{Issuer: "CDA"}, want: "needs an issuer and an id_type"},
		"empty param name": {
			reference: nswpayment.FeeReference{Issuer: "CDA", IDType: "fee_payment_ref", Params: map[string]string{"": "/exporter_id"}},
			want:      "params has an empty name",
		},
		"empty value name": {
			reference: nswpayment.FeeReference{Issuer: "CDA", IDType: "fee_payment_ref", Values: map[string]string{"": "01"}},
			want:      "values has an empty name",
		},
		"param in params and values": {
			reference: nswpayment.FeeReference{Issuer: "CDA", IDType: "fee_payment_ref", Params: exporterID, Values: map[string]string{"exporterId": "0002"}},
			want:      `names "exporterId" in both params and values`,
		},
		"relative pointer": {
			reference: nswpayment.FeeReference{Issuer: "CDA", IDType: "fee_payment_ref", Params: map[string]string{"exporterId": "0/exporter_id"}},
			want:      "params.exporterId",
		},
		"param is not a string": {
			reference: nswpayment.FeeReference{Issuer: "CDA", IDType: "fee_payment_ref", Params: exporterID},
			inputs:    map[string]any{"exporter_id": 2.0},
			want:      `param "exporterId" (/exporter_id) is a float64, not a string`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &fakePaymentService{resp: checkoutResponse()}
			fee := testFee()
			fee.Reference = &tc.reference

			err := paymentPlugin(svc, fee).Execute(paymentPluginCtx(tc.inputs), []byte(paymentProps))

			require.ErrorContains(t, err, tc.want)
			assert.False(t, svc.called, "the checkout opens only with a valid reference format")
		})
	}
}

// A reference a transaction already holds reaches the step as
// payment.ErrDuplicateReference; the step's retry generates a new one.
func TestPaymentPlugin_Execute_DuplicateReference(t *testing.T) {
	svc := &fakePaymentService{err: fmt.Errorf("failed to persist transaction: %w", payment.ErrDuplicateReference)}

	err := paymentPlugin(svc, testFee()).Execute(paymentPluginCtx(nil), []byte(paymentProps))

	require.ErrorIs(t, err, payment.ErrDuplicateReference)
}

// The step charges the fee its task code names: its amount, currency, service
// name and gateway metadata reach the checkout and the step's data. Any other
// property the artifact still carries is ignored.
func TestPaymentPlugin_Execute_ChargesTheNamedFee(t *testing.T) {
	svc := &fakePaymentService{resp: checkoutResponse()}
	other := nswpayment.Fee{TaskCode: "OTHER", ServiceName: "Other Fee", Amount: decimal.NewFromInt(1), Currency: "USD"}
	fee := nswpayment.Fee{
		TaskCode: "npqs_certificate_fee_payment", ServiceName: "Phytosanitary Certificate Fee",
		Amount: decimal.RequireFromString("2000.00"), Currency: "LKR",
		GatewayMetadata: map[string]string{"govpay_subinst_id": "002", "govpay_service_id": "001"},
	}
	ctx := paymentPluginCtx(nil)
	ctx.OutputNamespace = "payment"

	err := NewPaymentPlugin(svc, nswpayment.Fees{Fees: []nswpayment.Fee{other, fee}}).Execute(ctx,
		[]byte(`{"task_code":"npqs_certificate_fee_payment","amount":"1.00","currency":"USD"}`))

	require.ErrorIs(t, err, ErrSuspended)
	assert.True(t, svc.lastReq.Amount.Equal(decimal.NewFromInt(2000)), "charged %s", svc.lastReq.Amount)
	assert.Equal(t, "LKR", svc.lastReq.Currency)
	assert.Equal(t, "002", svc.lastReq.Metadata["govpay_subinst_id"])
	assert.Equal(t, "001", svc.lastReq.Metadata["govpay_service_id"])
	assert.Equal(t, "npqs_certificate_fee_payment", svc.lastReq.Metadata["task_code"])
	data := ctx.Record.Data["payment"].(map[string]any)
	assert.Equal(t, "Phytosanitary Certificate Fee", data["service_name"])
	assert.Equal(t, "npqs_certificate_fee_payment", data["service_type"])
}

func TestPaymentPlugin_Execute_RejectsUnknownOrMissingTaskCode(t *testing.T) {
	for name, tc := range map[string]struct {
		props string
		want  string
	}{
		"no task_code":      {props: `{}`, want: "plugin_properties.task_code is required"},
		"unknown task_code": {props: `{"task_code":"NOPE"}`, want: `no fee for task_code "NOPE"`},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &fakePaymentService{resp: checkoutResponse()}
			ctx := paymentPluginCtx(nil)

			err := paymentPlugin(svc, testFee()).Execute(ctx, []byte(tc.props))

			require.ErrorContains(t, err, tc.want)
			assert.False(t, svc.called, "no checkout opens without a configured fee")
			assert.NotEqual(t, "PENDING_PAYMENT", ctx.Record.State)
		})
	}
}
