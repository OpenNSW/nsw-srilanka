package authn

import coreauthn "github.com/OpenNSW/core/authn"

// Config is the authentication configuration this application supplies, loaded
// from the environment by cmd/server/config.
//
// It deliberately omits core/authn's claim-declaration fields: which claims to
// extract is this package's own business, not a per-deployment setting, so
// coreConfig declares them from the claim constants in principal.go.
type Config struct {
	JWKSURL               string
	Issuer                string
	Audience              string
	ClientIDs             []string
	InsecureSkipTLSVerify bool
}

// coreConfig maps Config onto core/authn's Config and declares the extra claims
// this package reads. Declaration and consumption live together on purpose: a
// claim surfaced on Principal but not declared here is silently never
// extracted.
//
// Required mirrors what the fixed schema used to guarantee before core/authn
// generalised its claim handling: email, ouId and ouHandle reject a token when
// absent (ouHandle in particular backs the user_records.ou_handle NOT NULL
// constraint and the task-ownership gate, so a missing value must fail at the
// token boundary rather than deeper in a request). phone_number is best-effort.
func (c Config) coreConfig() coreauthn.Config {
	return coreauthn.Config{
		JWKSURL:               c.JWKSURL,
		Issuer:                c.Issuer,
		Audience:              c.Audience,
		ClientIDs:             c.ClientIDs,
		InsecureSkipTLSVerify: c.InsecureSkipTLSVerify,
		UserClaims: coreauthn.ClaimSpec{
			Required: []string{claimEmail, claimOUID, claimOUHandle},
			Optional: []string{claimPhoneNumber},
		},
	}
}

// Validate reports whether the configuration is usable, including the claim
// declarations coreConfig adds.
func (c Config) Validate() error {
	return c.coreConfig().Validate()
}
