package payment

import (
	"context"
	"encoding/json"
	"testing"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapGovPayStatus(t *testing.T) {
	cases := map[string]struct {
		in      string
		want    corepayment.WebhookStatus
		wantErr bool
	}{
		"paid":      {in: "paid", want: corepayment.WebhookStatusSuccess},
		"completed": {in: "COMPLETED", want: corepayment.WebhookStatusSuccess},
		"success":   {in: "success", want: corepayment.WebhookStatusSuccess},
		"declined":  {in: "declined", want: corepayment.WebhookStatusFailed},
		"rejected":  {in: "REJECTED", want: corepayment.WebhookStatusFailed},
		"pending":   {in: " Pending ", want: corepayment.WebhookStatusPending}, // trims + case-insensitive
		"unknown":   {in: "weird", wantErr: true},
		"empty":     {in: "", wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := mapGovPayStatus(tc.in)
			if tc.wantErr {
				require.ErrorIs(t, err, corepayment.ErrUnsupportedWebhookStatus)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// updateBody is a GovPay+ update (webhook) request carrying the payment outcome
// as named data[] items.
func updateBody(refNo, status, amount, currency string) []byte {
	return []byte(`{
		"transactionID":"gw-tx-1",
		"subinstId":"s1",
		"serviceid":"sv1",
		"serviceName":"App Fee",
		"data":[
			{"seq":"1","paramName":"refNo","value":"` + refNo + `"},
			{"seq":"2","paramName":"amount","value":"` + amount + `"},
			{"seq":"3","paramName":"currency","value":"` + currency + `"},
			{"seq":"4","paramName":"status","value":"` + status + `"},
			{"seq":"5","paramName":"paymentMethod","value":"CC"}
		]
	}`)
}

func TestGovPay_ParseWebhook_NormalizesStatus(t *testing.T) {
	g := &GovPayGateway{}

	p, _, err := g.ParseWebhook(context.Background(), updateBody("TNSW1", "paid", "1500.00", "LKR"), nil)
	require.NoError(t, err)
	assert.Equal(t, "TNSW1", p.ReferenceNumber)
	assert.Equal(t, corepayment.WebhookStatusSuccess, p.Status)
	assert.Equal(t, "CC", p.PaymentMethod)
	assert.Equal(t, "LKR", p.Currency)
	assert.Equal(t, "gw-tx-1", p.GatewayTransactionID)
	assert.True(t, p.Amount.Equal(decimal.RequireFromString("1500.00")))
}

func TestGovPay_ParseWebhook_UnknownStatus(t *testing.T) {
	g := &GovPayGateway{}
	_, _, err := g.ParseWebhook(context.Background(), updateBody("TNSW1", "weird", "1500.00", "LKR"), nil)
	require.ErrorIs(t, err, corepayment.ErrUnsupportedWebhookStatus)
}

func TestGovPay_ParseWebhook_MissingRefNo(t *testing.T) {
	g := &GovPayGateway{}
	body := []byte(`{"transactionID":"gw-tx-1","data":[{"paramName":"status","value":"paid"}]}`)
	_, _, err := g.ParseWebhook(context.Background(), body, nil)
	require.Error(t, err)
}

func TestGovPay_ParseWebhook_InvalidJSON(t *testing.T) {
	g := &GovPayGateway{}
	_, _, err := g.ParseWebhook(context.Background(), []byte(`not json`), nil)
	require.Error(t, err)
}

func TestGovPay_ParseWebhook_Acknowledgement(t *testing.T) {
	g := &GovPayGateway{}
	body := updateBody("TNSW1", "paid", "1500.00", "LKR")

	_, resp, err := g.ParseWebhook(context.Background(), body, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.HTTPStatus)

	var out UpdateResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, "gw-tx-1", out.TransactionID)
	assert.Equal(t, "sv1", out.ServiceID)
	assert.Equal(t, "Success", out.Message)
	// Echoed data[] (5 items) + receipt number + status.
	require.Len(t, out.PaymentData, 7)
	assert.Equal(t, "Receipt Number", out.PaymentData[5].Placeholder)
	assert.Equal(t, "REC-gw-tx-1", out.PaymentData[5].InitialValue)
	assert.Equal(t, "Status", out.PaymentData[6].Placeholder)
}

func presentmentBody(refNo string) []byte {
	return []byte(`{
		"transactionID":"abc",
		"subinstId":"s1",
		"serviceid":"sv1",
		"serviceName":"App Fee",
		"data":[{"seq":"1","paramName":"refNo","value":"` + refNo + `"}]
	}`)
}

func TestGovPay_HandleValidateReference(t *testing.T) {
	g := &GovPayGateway{}
	reqData := presentmentBody("TNSW1")

	t.Run("payable", func(t *testing.T) {
		tx := &corepayment.ValidationTransaction{
			ReferenceNumber: "TNSW1",
			Amount:          decimal.RequireFromString("1500.00"),
			Currency:        "LKR",
		}
		resp, err := g.HandleValidateReference(context.Background(), tx, true, reqData)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.HTTPStatus)

		var out PresentmentResponse
		require.NoError(t, json.Unmarshal(resp.Payload, &out))
		assert.Equal(t, "Success", out.Message)
		assert.Equal(t, "abc", out.TransactionID)
		assert.Equal(t, "sv1", out.ServiceID)
		require.NotEmpty(t, out.PresentmentData)
		assert.Equal(t, "TNSW1", out.PresentmentData[0].InitialValue)
		assert.True(t, out.PresentmentData[0].IsPaymentReference)
		assert.Equal(t, "refNo", out.PresentmentData[0].ReturnParam)
	})

	t.Run("unknown reference", func(t *testing.T) {
		resp, err := g.HandleValidateReference(context.Background(), nil, false, reqData)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.HTTPStatus)

		var out ErrorResponse
		require.NoError(t, json.Unmarshal(resp.Payload, &out))
		assert.Equal(t, "invalid_reference", out.Error)
	})

	t.Run("not payable", func(t *testing.T) {
		resp, err := g.HandleValidateReference(context.Background(), &corepayment.ValidationTransaction{ReferenceNumber: "TNSW1"}, false, reqData)
		require.NoError(t, err)
		assert.Equal(t, 409, resp.HTTPStatus)

		var out ErrorResponse
		require.NoError(t, json.Unmarshal(resp.Payload, &out))
		assert.Equal(t, "not_payable", out.Error)
	})
}

func TestGovPay_ExtractReferenceNumber(t *testing.T) {
	g := &GovPayGateway{}

	ref, err := g.ExtractReferenceNumber(context.Background(), presentmentBody("TNSW1"))
	require.NoError(t, err)
	assert.Equal(t, "TNSW1", ref)

	_, err = g.ExtractReferenceNumber(context.Background(), []byte(`{"transactionID":"abc"}`))
	require.Error(t, err)

	_, err = g.ExtractReferenceNumber(context.Background(), presentmentBody("TN SW!"))
	require.Error(t, err)
}

func TestGovPay_CreateSession(t *testing.T) {
	g := &GovPayGateway{}
	resp, err := g.CreateSession(context.Background(), corepayment.SessionRequest{})
	require.NoError(t, err)
	assert.Equal(t, corepayment.FlowTypeInstruction, resp.Type)
	assert.NotEmpty(t, resp.Instructions)
}

func TestNewGovPayGateway(t *testing.T) {
	gw, err := NewGovPayGateway([]byte(`{"BaseURL":"https://sandbox.govpay.lk"}`))
	require.NoError(t, err)
	g, ok := gw.(*GovPayGateway)
	require.True(t, ok)
	assert.Equal(t, "https://sandbox.govpay.lk", g.cfg.BaseURL)

	_, err = NewGovPayGateway([]byte(`not json`))
	require.Error(t, err)
}

func TestGovPay_ExtractReferenceNumber_InvalidJSON(t *testing.T) {
	g := &GovPayGateway{}
	_, err := g.ExtractReferenceNumber(context.Background(), []byte(`not json`))
	require.Error(t, err)
}

func TestGovPay_HandleValidateReference_InvalidJSON(t *testing.T) {
	g := &GovPayGateway{}
	_, err := g.HandleValidateReference(context.Background(), nil, true, []byte(`not json`))
	require.Error(t, err)
}
