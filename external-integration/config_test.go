package integrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	slpawebhook "github.com/OpenNSW/nsw-srilanka/external-integration/slpa/webhook"
)

func TestConfig_Validate(t *testing.T) {
	valid := Config{SLPAWebhookSecret: "a-secret-shared-with-slpa"}
	require.NoError(t, valid.Validate())

	// Each integration answers for itself; this only names which one refused, so
	// a deployment reading the message knows where to look.
	err := Config{}.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, slpawebhook.ErrNoSecret)
	assert.Contains(t, err.Error(), "slpa webhook")
	assert.Contains(t, err.Error(), "SLPA_WEBHOOK_SECRET", "the message names the variable to set")
}
