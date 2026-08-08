package govpay

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shared bodies in govpay_test.go post subinstId "s1" and serviceid "sv1".

const (
	wantSubInst = "s1"
	wantService = "sv1"
)

func payableTx(metadata map[string]string) *corepayment.ValidationTransaction {
	return &corepayment.ValidationTransaction{
		ReferenceNumber: "TNSW1",
		Amount:          decimal.RequireFromString("1500.00"),
		Currency:        "LKR",
		Metadata:        metadata,
	}
}

func configured(subInstID, serviceID string) map[string]string {
	return map[string]string{MetadataSubInstID: subInstID, MetadataServiceID: serviceID}
}

func staticResolver(identity ExpectedIdentity, found bool, err error) IdentityResolver {
	return func(context.Context, string) (ExpectedIdentity, bool, error) {
		return identity, found, err
	}
}

// configuredGateway is a gateway wired the way production wires it: able to
// resolve the identity the shared test bodies are posted under. Tests that are
// not about the identity check use this so they exercise the happy path.
func configuredGateway() *GovPayGateway {
	return &GovPayGateway{
		resolveIdentity: staticResolver(ExpectedIdentity{SubInstID: wantSubInst, ServiceID: wantService}, true, nil),
	}
}

// -----------------------------------------------------------------------------
// Presence: both ids are mandatory on the wire, for both endpoints.
// -----------------------------------------------------------------------------

func TestGovPay_RequiresIdentityFieldsOnTheWire(t *testing.T) {
	bodies := map[string]string{
		"both missing":     `{"transactionID":"gw-1","serviceName":"Fee","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"subinstId absent": `{"transactionID":"gw-1","serviceid":"sv1","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"serviceid absent": `{"transactionID":"gw-1","subinstId":"s1","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"subinstId blank":  `{"transactionID":"gw-1","subinstId":"  ","serviceid":"sv1","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"serviceid blank":  `{"transactionID":"gw-1","subinstId":"s1","serviceid":"","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
	}

	for name, body := range bodies {
		t.Run("presentment/"+name, func(t *testing.T) {
			g := &GovPayGateway{}
			_, err := g.HandleValidateReference(context.Background(), payableTx(configured(wantSubInst, wantService)), true, json.RawMessage(body))
			require.Error(t, err, "a call without both ids must not be processed")
		})

		t.Run("update/"+name, func(t *testing.T) {
			g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{SubInstID: wantSubInst, ServiceID: wantService}, true, nil)}
			_, _, err := g.ParseWebhook(context.Background(), []byte(body), nil)
			require.Error(t, err)
		})
	}
}

// -----------------------------------------------------------------------------
// Presentment (validate)
// -----------------------------------------------------------------------------

func TestGovPay_HandleValidateReference_IdentityMatching(t *testing.T) {
	reqData := presentmentBody("TNSW1")

	tests := []struct {
		name       string
		metadata   map[string]string
		wantStatus int
		wantError  string
	}{
		{
			name:       "both ids match",
			metadata:   configured(wantSubInst, wantService),
			wantStatus: 200,
		},
		{
			// The ids are onboarding-time constants typed into configuration by
			// hand, so casing and stray spaces must not reject a valid call.
			name:       "match ignores case and surrounding space",
			metadata:   configured("  S1 ", "SV1"),
			wantStatus: 200,
		},
		{
			name:       "wrong sub-institution",
			metadata:   configured("other", wantService),
			wantStatus: 404,
			wantError:  "invalid_reference",
		},
		{
			name:       "wrong service",
			metadata:   configured(wantSubInst, "other"),
			wantStatus: 404,
			wantError:  "invalid_reference",
		},
		{
			// Configuration is mandatory: an undeclared fee is refused, not
			// waved through unchecked.
			name:       "fee declared neither id",
			metadata:   nil,
			wantStatus: 500,
			wantError:  "configuration_error",
		},
		{
			name:       "fee declared only the service id",
			metadata:   map[string]string{MetadataServiceID: wantService},
			wantStatus: 500,
			wantError:  "configuration_error",
		},
		{
			name:       "fee declared only the sub-institution id",
			metadata:   map[string]string{MetadataSubInstID: wantSubInst},
			wantStatus: 500,
			wantError:  "configuration_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &GovPayGateway{}
			resp, err := g.HandleValidateReference(context.Background(), payableTx(tc.metadata), true, reqData)
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, resp.HTTPStatus)

			if tc.wantError == "" {
				return
			}
			var out ErrorResponse
			require.NoError(t, json.Unmarshal(resp.Payload, &out))
			assert.Equal(t, tc.wantError, out.Error)
			// Neither response reveals anything about the reference itself.
			assert.NotContains(t, out.Message, "TNSW1")
		})
	}
}

// A mismatched identity must be rejected before payability is considered, so
// the response cannot reveal that the reference exists and is already settled.
func TestGovPay_HandleValidateReference_MismatchOutranksPayability(t *testing.T) {
	g := &GovPayGateway{}
	tx := payableTx(configured("other", wantService))

	resp, err := g.HandleValidateReference(context.Background(), tx, false, presentmentBody("TNSW1"))
	require.NoError(t, err)
	assert.Equal(t, 404, resp.HTTPStatus, "identity is checked before payability")

	var out ErrorResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, "invalid_reference", out.Error, "must not disclose not_payable")
}

// -----------------------------------------------------------------------------
// Update (webhook)
// -----------------------------------------------------------------------------

func TestGovPay_ParseWebhook_IdentityMatching(t *testing.T) {
	body := updateBody("TNSW1", "paid", "1500.00", "LKR")

	t.Run("matching identity is accepted", func(t *testing.T) {
		g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{SubInstID: wantSubInst, ServiceID: wantService}, true, nil)}
		p, _, err := g.ParseWebhook(context.Background(), body, nil)
		require.NoError(t, err)
		assert.Equal(t, corepayment.WebhookStatusSuccess, p.Status)
	})

	t.Run("mismatched identity is rejected", func(t *testing.T) {
		g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{SubInstID: wantSubInst, ServiceID: "other"}, true, nil)}
		_, _, err := g.ParseWebhook(context.Background(), body, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityMismatch)
		assert.Contains(t, err.Error(), "TNSW1")
	})

	t.Run("an undeclared fee cannot be settled", func(t *testing.T) {
		g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{}, true, nil)}
		_, _, err := g.ParseWebhook(context.Background(), body, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityNotConfigured)
	})

	t.Run("a partly declared fee cannot be settled", func(t *testing.T) {
		g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{ServiceID: wantService}, true, nil)}
		_, _, err := g.ParseWebhook(context.Background(), body, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityNotConfigured)
	})

	t.Run("a gateway with no resolver refuses to settle", func(t *testing.T) {
		// Without a resolver the identity cannot be checked at all, so the
		// notification is refused rather than accepted unverified.
		g := &GovPayGateway{}
		_, _, err := g.ParseWebhook(context.Background(), body, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityNotConfigured)
	})

	t.Run("unknown reference passes through to the service", func(t *testing.T) {
		// The service looks the reference up immediately afterwards and owns
		// that failure; reporting it here too would surface it two ways.
		g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{}, false, nil)}
		_, _, err := g.ParseWebhook(context.Background(), body, nil)
		require.NoError(t, err)
	})

	t.Run("resolver failure is surfaced", func(t *testing.T) {
		boom := errors.New("db down")
		g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{}, false, boom)}
		_, _, err := g.ParseWebhook(context.Background(), body, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("identity is checked before the status is trusted", func(t *testing.T) {
		// An unparseable status would normally be the reported failure; the
		// identity check must fire first so a receipt for another service can
		// never reach the status mapping.
		g := &GovPayGateway{resolveIdentity: staticResolver(ExpectedIdentity{SubInstID: wantSubInst, ServiceID: "other"}, true, nil)}
		_, _, err := g.ParseWebhook(context.Background(), updateBody("TNSW1", "weird", "1500.00", "LKR"), nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityMismatch)
	})
}

func TestGovPay_NewGovPayGatewayFactory_WiresResolver(t *testing.T) {
	factory := NewGovPayGatewayFactory(staticResolver(ExpectedIdentity{SubInstID: wantSubInst, ServiceID: "other"}, true, nil))
	gw, err := factory(json.RawMessage(`{}`))
	require.NoError(t, err)

	_, _, err = gw.ParseWebhook(context.Background(), updateBody("TNSW1", "paid", "1500.00", "LKR"), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrIdentityMismatch)

	// The bare constructor wires no resolver, so it cannot settle a webhook.
	plain, err := NewGovPayGateway(json.RawMessage(`{}`))
	require.NoError(t, err)
	_, _, err = plain.ParseWebhook(context.Background(), updateBody("TNSW1", "paid", "1500.00", "LKR"), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrIdentityNotConfigured)
}
