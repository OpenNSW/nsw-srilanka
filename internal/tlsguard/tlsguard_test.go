package tlsguard

import "testing"

func TestIsDevEnvironment(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"development", true},
		{"Development", true},
		{" development ", true},
		{"production", false},
		{"", false},
		{"staging", false},
	}
	for _, c := range cases {
		t.Run(c.val, func(t *testing.T) {
			t.Setenv(EnvKey, c.val)
			if got := IsDevEnvironment(); got != c.want {
				t.Fatalf("IsDevEnvironment() with %s=%q = %v, want %v", EnvKey, c.val, got, c.want)
			}
		})
	}
}

func TestGuard_AllowsInDevelopment(t *testing.T) {
	t.Setenv(EnvKey, "development")
	if err := Guard("test"); err != nil {
		t.Fatalf("expected nil in development, got %v", err)
	}
}

func TestGuard_FailsClosedOutsideDevelopment(t *testing.T) {
	for _, val := range []string{"", "production", "staging"} {
		t.Run(val, func(t *testing.T) {
			t.Setenv(EnvKey, val)
			if err := Guard("AUTH_JWKS_INSECURE_SKIP_VERIFY"); err == nil {
				t.Fatalf("expected error when %s=%q, got nil", EnvKey, val)
			}
		})
	}
}
