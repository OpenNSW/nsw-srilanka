package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/OpenNSW/core/notifications/providers"
	"github.com/OpenNSW/nsw-srilanka/external-integration/pingmailer"
)

func emailConfig(t *testing.T, doc string) EmailProviderConfig {
	t.Helper()
	var c EmailProviderConfig
	require.NoError(t, yaml.Unmarshal([]byte(doc), &c))
	return c
}

// A config without a type keeps the provider it had: core's, from baseURL and
// token.
func TestEmailProviderConfig_DefaultsToTheHTTPProvider(t *testing.T) {
	c := emailConfig(t, `
baseURL: https://email.example.com
token: email-token
`)

	p, err := c.Build()

	require.NoError(t, err)
	assert.IsType(t, &providers.EmailProvider{}, p)
}

func TestEmailProviderConfig_PingMailer(t *testing.T) {
	c := emailConfig(t, `
type: pingmailer
pingmailer:
  baseURL: http://api-server.mail.svc.cluster.local:8000
  allowInsecureHTTP: true
  auth:
    type: oauth2
    oauth2:
      tokenURL: https://idp.example.com/oauth2/token
      clientID: pingmailer-client
      clientSecret: secret
      endpointParams:
        resource: https://mail.example.com
  smtpHost: mail.example.com
  smtpPort: 587
  sender: no-reply@example.com
`)

	p, err := c.Build()

	require.NoError(t, err)
	assert.IsType(t, &pingmailer.Provider{}, p)
}

func TestEmailProviderConfig_Refused(t *testing.T) {
	pm := `
  baseURL: https://mail-api.example.com
  auth:
    type: oauth2
    oauth2:
      tokenURL: https://idp.example.com/oauth2/token
      clientID: c
      clientSecret: s
      insecureSkipTLSVerify: %s
  smtpHost: mail.example.com
  smtpPort: 587
  sender: no-reply@example.com
`
	for name, tc := range map[string]struct {
		doc     string
		wantErr string
	}{
		"unknown type":            {"type: smtp\n", "unknown type"},
		"token with pingmailer":   {"type: pingmailer\ntoken: t\npingmailer:" + fmt.Sprintf(pm, "false"), "not used with type pingmailer"},
		"skip TLS outside of dev": {"type: pingmailer\npingmailer:" + fmt.Sprintf(pm, "true"), "insecureSkipTLSVerify"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			_, err := emailConfig(t, tc.doc).Build()
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}
