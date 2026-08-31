package webhook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deployment without the secret cannot hear SLPA's decisions, so it must not
// start: it would accept service orders whose answer never arrives.
func TestConfig_Validate(t *testing.T) {
	require.NoError(t, Config{Secret: "a-secret-shared-with-slpa"}.Validate())

	for name, secret := range map[string]string{"empty": "", "blank": "   \n"} {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, Config{Secret: secret}.Validate(), ErrNoSecret)
		})
	}
}
