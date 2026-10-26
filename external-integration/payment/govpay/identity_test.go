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

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// The shared bodies in govpay_test.go post subinstId "002" and serviceid "001".

const (
	wantSubInst = "002"
	wantService = "001"
)

func payableTx(mapping json.RawMessage) *corepayment.ValidationTransaction {
	return &corepayment.ValidationTransaction{
		ReferenceNumber: "TNSW1",
		Amount:          decimal.RequireFromString("1500.00"),
		Currency:        "LKR",
		PayeeMapping:    mapping,
	}
}

// configured is the govpay payee mapping for the given ids.
func configured(subInstID, serviceID string) json.RawMessage {
	mapping, _ := json.Marshal(map[string]string{"subinst_id": subInstID, "service_id": serviceID})
	return mapping
}

// staticLookup stands in for the payee lookup core/payment hands ParseWebhook.
func staticLookup(mapping json.RawMessage, found bool, err error) corepayment.PayeeLookup {
	return func(context.Context, string) (json.RawMessage, bool, error) {
		return mapping, found, err
	}
}

// -----------------------------------------------------------------------------
// Presence: both ids are mandatory on the wire, for both endpoints.
// -----------------------------------------------------------------------------

func TestGovPay_RequiresIdentityFieldsOnTheWire(t *testing.T) {
	bodies := map[string]string{
		"both missing":     `{"transactionID":"gw1","serviceName":"Fee","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"subinstId absent": `{"transactionID":"gw1","serviceid":"001","serviceName":"Fee","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"serviceid absent": `{"transactionID":"gw1","subinstId":"002","serviceName":"Fee","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"subinstId blank":  `{"transactionID":"gw1","subinstId":"  ","serviceid":"001","serviceName":"Fee","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
		"serviceid blank":  `{"transactionID":"gw1","subinstId":"002","serviceid":"","serviceName":"Fee","data":[{"seq":"1","paramName":"refNo","value":"TNSW1"}]}`,
	}

	for name, body := range bodies {
		t.Run("presentment/"+name, func(t *testing.T) {
			g := newTestGateway()
			_, err := g.validateEnc(t, payableTx(configured(wantSubInst, wantService)), true, json.RawMessage(body))
			require.Error(t, err, "a call without both ids must not be processed")
		})

		t.Run("update/"+name, func(t *testing.T) {
			g := newTestGateway()
			_, _, err := g.parseWebhookEnc(t, []byte(body), nil)
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
		mapping    json.RawMessage
		wantStatus int
		wantError  string
	}{
		{
			name:       "both ids match",
			mapping:    configured(wantSubInst, wantService),
			wantStatus: 200,
		},
		{
			// The ids are onboarding-time constants typed into configuration by
			// hand, so stray surrounding spaces must not reject a valid call.
			name:       "match trims surrounding configuration whitespace",
			mapping:    configured(" 002 ", " 001 "),
			wantStatus: 200,
		},
		{
			name:       "wrong sub-institution",
			mapping:    configured("other", wantService),
			wantStatus: 404,
			wantError:  "invalid_reference",
		},
		{
			name:       "wrong service",
			mapping:    configured(wantSubInst, "other"),
			wantStatus: 404,
			wantError:  "invalid_reference",
		},
		{
			name:       "malformed mapping",
			mapping:    json.RawMessage(`["002","001"]`),
			wantStatus: 500,
			wantError:  "configuration_error",
		},
		{
			// Configuration is mandatory: an unmapped payee is refused, not
			// waved through unchecked.
			name:       "payee mapped to neither id",
			mapping:    nil,
			wantStatus: 500,
			wantError:  "configuration_error",
		},
		{
			name:       "payee mapped to only the service id",
			mapping:    json.RawMessage(`{"service_id":"001"}`),
			wantStatus: 500,
			wantError:  "configuration_error",
		},
		{
			name:       "payee mapped to only the sub-institution id",
			mapping:    json.RawMessage(`{"subinst_id":"002"}`),
			wantStatus: 500,
			wantError:  "configuration_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGateway()
			resp, err := g.validateEnc(t, payableTx(tc.mapping), true, reqData)
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
	g := newTestGateway()
	tx := payableTx(configured("other", wantService))

	resp, err := g.validateEnc(t, tx, false, presentmentBody("TNSW1"))
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
	g := newTestGateway()

	t.Run("matching identity is accepted", func(t *testing.T) {
		p, _, err := g.parseWebhookEncWith(t, body, staticLookup(configured(wantSubInst, wantService), true, nil))
		require.NoError(t, err)
		assert.Equal(t, corepayment.WebhookStatusSuccess, p.Status)
	})

	t.Run("mismatched identity is rejected", func(t *testing.T) {
		_, _, err := g.parseWebhookEncWith(t, body, staticLookup(configured(wantSubInst, "other"), true, nil))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityMismatch)
		assert.Contains(t, err.Error(), "TNSW1")
	})

	t.Run("an unmapped payee cannot be settled", func(t *testing.T) {
		_, _, err := g.parseWebhookEncWith(t, body, staticLookup(json.RawMessage(`{}`), true, nil))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityNotConfigured)
	})

	t.Run("a partly mapped payee cannot be settled", func(t *testing.T) {
		_, _, err := g.parseWebhookEncWith(t, body, staticLookup(json.RawMessage(`{"service_id":"001"}`), true, nil))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityNotConfigured)
	})

	t.Run("no lookup refuses to settle", func(t *testing.T) {
		// Without a lookup the identity cannot be checked at all, so the
		// notification is refused rather than accepted unverified.
		_, _, err := g.parseWebhookEncWith(t, body, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIdentityNotConfigured)
	})

	t.Run("unknown reference passes through to the service", func(t *testing.T) {
		// The service looks the reference up immediately afterwards and owns
		// that failure; reporting it here too would surface it two ways.
		_, _, err := g.parseWebhookEncWith(t, body, staticLookup(nil, false, nil))
		require.NoError(t, err)
	})

	t.Run("lookup failure is surfaced", func(t *testing.T) {
		boom := errors.New("db down")
		_, _, err := g.parseWebhookEncWith(t, body, staticLookup(nil, false, boom))
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("a payee no longer mapped is surfaced", func(t *testing.T) {
		_, _, err := g.parseWebhookEncWith(t, body, staticLookup(nil, false, corepayment.ErrPayeeNotMapped))
		require.ErrorIs(t, err, corepayment.ErrPayeeNotMapped)
	})
}

// TestValidatePayeeMapping covers the check that keeps a payee without a full
// GovPay+ identity from ever reaching a checkout: the callbacks above compare the
// posted identity against the mapping, so an incomplete mapping is unsettleable
// and must stop startup rather than fail at settlement.
func TestValidatePayeeMapping(t *testing.T) {
	g := newTestGateway()

	t.Run("both ids mapped", func(t *testing.T) {
		require.NoError(t, g.ValidatePayeeMapping(configured(wantSubInst, wantService)))
	})

	for name, mapping := range map[string]string{
		"neither id":               `{"merchant_id":"M1"}`,
		"only the sub-institution": `{"subinst_id":"002"}`,
		"only the service":         `{"service_id":"001"}`,
		"whitespace id":            `{"subinst_id":"  ","service_id":"001"}`,
		"not an object":            `"002/001"`,
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			err := g.ValidatePayeeMapping(json.RawMessage(mapping))
			require.ErrorIs(t, err, ErrIdentityNotConfigured)
		})
	}

	t.Run("names both keys", func(t *testing.T) {
		err := g.ValidatePayeeMapping(json.RawMessage(`{}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subinst_id")
		assert.Contains(t, err.Error(), "service_id")
	})
}

// clientCtx builds the request context the authn middleware leaves behind for a
// machine caller.
func clientCtx(clientID string) context.Context {
	return authn.ContextWithPrincipal(context.Background(), &authn.Principal{Kind: authn.KindClient, ClientID: clientID})
}

// TestVerifyWebhook covers the check that keeps a machine client holding the
// payment webhook scopes from acting for a gateway it is not GovPay+ for.
func TestVerifyWebhook(t *testing.T) {
	g := &GovPayGateway{cfg: Config{WebhookClientID: "GOVPAY_TO_NSW"}}

	t.Run("the configured client is accepted", func(t *testing.T) {
		require.NoError(t, g.VerifyWebhook(clientCtx("GOVPAY_TO_NSW"), nil, nil))
	})

	// The scopes are granted per client, not per gateway, so another client
	// holding them must not be able to settle a GovPay+ transaction.
	t.Run("another authenticated client is rejected", func(t *testing.T) {
		err := g.VerifyWebhook(clientCtx("SLCE_TO_NSW"), nil, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, corepayment.ErrWebhookVerificationFailed,
			"a wrong caller is a rejection, so it must map to 401 rather than a retryable 500")
		assert.Contains(t, err.Error(), "SLCE_TO_NSW")
	})

	t.Run("a call with no machine principal is rejected", func(t *testing.T) {
		err := g.VerifyWebhook(context.Background(), nil, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, corepayment.ErrWebhookVerificationFailed)
	})

	t.Run("a user token is rejected", func(t *testing.T) {
		ctx := authn.ContextWithPrincipal(context.Background(), &authn.Principal{Kind: authn.KindUser, IDPUserID: "u1"})
		err := g.VerifyWebhook(ctx, nil, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, corepayment.ErrWebhookVerificationFailed)
	})

	// Missing configuration is not evidence about the caller. Reporting it as a
	// verification failure would answer 401, burning the gateway's retry budget
	// on a request that could succeed once the deployment is fixed.
	t.Run("an unconfigured client id is operational, not a rejection", func(t *testing.T) {
		unconfigured := &GovPayGateway{}
		err := unconfigured.VerifyWebhook(clientCtx("GOVPAY_TO_NSW"), nil, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrWebhookClientNotConfigured)
		assert.NotErrorIs(t, err, corepayment.ErrWebhookVerificationFailed)
	})
}
