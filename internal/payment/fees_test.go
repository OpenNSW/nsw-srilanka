package payment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMethodsFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payment_methods.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// Each method's fees sit in its config, beside the settings its gateway
// reads; a method without a fees list has none.
func TestLoadFees(t *testing.T) {
	path := writeMethodsFile(t, `{
		"version": "1.0",
		"methods": [
			{"id": "govpay", "is_active": true, "config": {
				"webhook_client_id": "GOVPAY_TO_NSW",
				"fees": [{
					"task_code": "example_fee_payment_v1",
					"gateway_metadata": {"govpay_subinst_id": "002", "govpay_service_id": "001"}
				}]
			}},
			{"id": "other", "is_active": true}
		]
	}`)

	fees, err := LoadFees(path)

	require.NoError(t, err)
	metadata, ok := fees.GatewayMetadata("govpay", "example_fee_payment_v1")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"govpay_subinst_id": "002", "govpay_service_id": "001"}, metadata)
	_, ok = fees.GatewayMetadata("govpay", "other")
	assert.False(t, ok)
	_, ok = fees.GatewayMetadata("other", "example_fee_payment_v1")
	assert.False(t, ok, "a fee belongs to the method that lists it")
}

// An agency's payment methods file has no fees.
func TestLoadFees_NoFees(t *testing.T) {
	fees, err := LoadFees(writeMethodsFile(t, `{"version": "1.0", "methods": []}`))

	require.NoError(t, err)
	assert.Empty(t, fees)
}

func TestLoadFees_Rejects(t *testing.T) {
	for name, tc := range map[string]struct {
		fees string
		want string
	}{
		"no task code": {fees: `[{"gateway_metadata": {"k": "v"}}]`, want: `method "govpay": fees[0].task_code is required`},
		"duplicate":    {fees: `[{"task_code": "A"}, {"task_code": "A"}]`, want: `method "govpay": task_code "A" is declared more than once`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadFees(writeMethodsFile(t, `{"methods": [{"id": "govpay", "config": {"fees": `+tc.fees+`}}]}`))

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestLoadFees_MissingFile(t *testing.T) {
	_, err := LoadFees(filepath.Join(t.TempDir(), "missing.json"))

	require.Error(t, err)
}
