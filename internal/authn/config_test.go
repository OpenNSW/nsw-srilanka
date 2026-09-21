package authn

import (
	"slices"
	"testing"
)

func validConfig() Config {
	return Config{
		JWKSURL:   "https://idp.example.com/oauth2/jwks",
		Issuer:    "https://idp.example.com",
		Audience:  "https://api.nsw-srilanka.local",
		ClientIDs: []string{"TRADER_PORTAL_APP"},
	}
}

// Every claim Principal surfaces must also be declared to core/authn, or it is
// silently never extracted and the field reads as empty at runtime. This test is
// the link between the two halves.
func TestConfig_DeclaresEveryClaimPrincipalReads(t *testing.T) {
	spec := validConfig().coreConfig().UserClaims
	declared := slices.Concat(spec.Required, spec.Optional)

	for _, claim := range []string{claimEmail, claimPhoneNumber, claimOUID, claimOUHandle} {
		if !slices.Contains(declared, claim) {
			t.Errorf("claim %q is read by principalFromAuthContext but never declared to core/authn", claim)
		}
	}
}

// ouHandle backs the task-ownership gate and the user_records.ou_handle NOT NULL
// constraint, so a token missing it must be rejected at the auth boundary rather
// than surfacing as a database error deeper in the request.
func TestConfig_OUHandleIsRequired(t *testing.T) {
	spec := validConfig().coreConfig().UserClaims
	if !slices.Contains(spec.Required, claimOUHandle) {
		t.Errorf("%q must be a required claim, got required=%v optional=%v", claimOUHandle, spec.Required, spec.Optional)
	}
	if !slices.Contains(spec.Optional, claimPhoneNumber) {
		t.Errorf("%q should be optional, got required=%v optional=%v", claimPhoneNumber, spec.Required, spec.Optional)
	}
}

func TestConfig_Validate(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("expected a valid config, got %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing jwks url", func(c *Config) { c.JWKSURL = "" }},
		{"missing issuer", func(c *Config) { c.Issuer = "" }},
		{"missing audience", func(c *Config) { c.Audience = "" }},
		{"missing client ids", func(c *Config) { c.ClientIDs = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}
