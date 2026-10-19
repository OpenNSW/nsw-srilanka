package payment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/core/refid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// npqsFormats defines the one reference format the test fees name.
var npqsFormats = refid.Config{Issuers: []refid.IssuerConfig{
	{Issuer: "NPQS", Formats: []refid.FormatConfig{{IDType: "npqs_payment_ref"}}},
}}

func writeMethodsFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payment_methods.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// The fees sit beside the gateway methods, which LoadFees leaves to
// core/payment.
func TestLoadFees(t *testing.T) {
	path := writeMethodsFile(t, `{
		"version": "1.0",
		"methods": [{"id": "govpay", "is_active": true}],
		"fees": [{
			"task_code": "npqs_certificate_fee_payment",
			"service_name": "Phytosanitary Certificate Fee",
			"amount": "2000.00",
			"currency": "LKR",
			"gateway_metadata": {"govpay_subinst_id": "002", "govpay_service_id": "001"},
			"reference": {"issuer": "NPQS", "id_type": "npqs_payment_ref"}
		}]
	}`)

	fees, err := LoadFees(path, npqsFormats)

	require.NoError(t, err)
	fee, ok := fees.Lookup("npqs_certificate_fee_payment")
	require.True(t, ok)
	assert.Equal(t, "Phytosanitary Certificate Fee", fee.ServiceName)
	assert.Equal(t, "2000", fee.Amount.String())
	assert.Equal(t, "002", fee.GatewayMetadata["govpay_subinst_id"])
	assert.Equal(t, &FeeReference{Issuer: "NPQS", IDType: "npqs_payment_ref"}, fee.Reference)
	_, ok = fees.Lookup("other")
	assert.False(t, ok)
}

// An agency's payment methods file has no fees.
func TestLoadFees_NoFees(t *testing.T) {
	fees, err := LoadFees(writeMethodsFile(t, `{"version": "1.0", "methods": []}`), refid.Config{})

	require.NoError(t, err)
	assert.Empty(t, fees.Fees)
}

func TestLoadFees_Rejects(t *testing.T) {
	for name, tc := range map[string]struct {
		fees string
		want string
	}{
		"no task code":    {fees: `[{"service_name": "Fee", "currency": "LKR"}]`, want: "fees[0].task_code is required"},
		"duplicate":       {fees: `[{"task_code": "A", "service_name": "Fee", "currency": "LKR"}, {"task_code": "A", "service_name": "Fee", "currency": "LKR"}]`, want: `task_code "A" is declared more than once`},
		"no service name": {fees: `[{"task_code": "A", "currency": "LKR"}]`, want: `fee "A": service_name is required`},
		"no currency":     {fees: `[{"task_code": "A", "service_name": "Fee"}]`, want: `fee "A": currency is required`},
		"negative amount": {fees: `[{"task_code": "A", "service_name": "Fee", "currency": "LKR", "amount": "-1"}]`, want: "amount must be positive"},
		"reference without id_type": {
			fees: `[{"task_code": "A", "service_name": "Fee", "currency": "LKR", "reference": {"issuer": "NPQS"}}]`,
			want: "reference needs an issuer and an id_type",
		},
		"undefined reference format": {
			fees: `[{"task_code": "A", "service_name": "Fee", "currency": "LKR", "reference": {"issuer": "NOPE", "id_type": "ref"}}]`,
			want: "reference format NOPE/ref is not defined in config.yaml refid",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadFees(writeMethodsFile(t, `{"methods": [], "fees": `+tc.fees+`}`), npqsFormats)

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestLoadFees_MissingFile(t *testing.T) {
	_, err := LoadFees(filepath.Join(t.TempDir(), "missing.json"), refid.Config{})

	require.Error(t, err)
}
