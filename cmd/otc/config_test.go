package main

import (
	"strings"
	"testing"
)

// --- getEnvOrDefault ---

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("unset key returns default", func(t *testing.T) {
		got := getEnvOrDefault("__UNSET_KEY_XYZ__", "fallback")
		if got != "fallback" {
			t.Errorf("got %q, want %q", got, "fallback")
		}
	})
	t.Run("set key returns trimmed value", func(t *testing.T) {
		t.Setenv("__TEST_ENV__", "  hello  ")
		got := getEnvOrDefault("__TEST_ENV__", "fallback")
		if got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})
	t.Run("whitespace-only value returns default", func(t *testing.T) {
		t.Setenv("__TEST_ENV_WS__", "   ")
		got := getEnvOrDefault("__TEST_ENV_WS__", "fallback")
		if got != "fallback" {
			t.Errorf("got %q, want %q", got, "fallback")
		}
	})
}

// --- getIntEnvOrDefault ---

func TestGetIntEnvOrDefault(t *testing.T) {
	t.Run("unset key returns default", func(t *testing.T) {
		got := getIntEnvOrDefault("__UNSET_INT__", 42)
		if got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})
	t.Run("valid int value", func(t *testing.T) {
		t.Setenv("__TEST_INT__", "9090")
		got := getIntEnvOrDefault("__TEST_INT__", 42)
		if got != 9090 {
			t.Errorf("got %d, want 9090", got)
		}
	})
	t.Run("invalid string returns default", func(t *testing.T) {
		t.Setenv("__TEST_INT_BAD__", "not-a-number")
		got := getIntEnvOrDefault("__TEST_INT_BAD__", 42)
		if got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})
	t.Run("whitespace-only returns default", func(t *testing.T) {
		t.Setenv("__TEST_INT_WS__", "   ")
		got := getIntEnvOrDefault("__TEST_INT_WS__", 42)
		if got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})
}

// --- Load ---

func TestLoad_Defaults(t *testing.T) {
	// DB_PASSWORD has no default and is required — set it explicitly.
	// All other env vars are cleared so defaults apply.
	envsToClear := []string{
		"DB_HOST", "DB_PORT", "DB_USERNAME", "DB_NAME", "DB_SSLMODE",
		"DB_MAX_IDLE_CONNS", "DB_MAX_OPEN_CONNS", "DB_MAX_CONN_LIFETIME_SECONDS",
	}
	for _, k := range envsToClear {
		t.Setenv(k, "")
	}
	t.Setenv("DB_PASSWORD", "testpassword")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"Database.Host", cfg.Database.Host, "localhost"},
		{"Database.Port", cfg.Database.Port, 5432},
		{"Database.Username", cfg.Database.Username, "postgres"},
		{"Database.Password", cfg.Database.Password, "testpassword"},
		{"Database.Name", cfg.Database.Name, "nsw_db"},
		{"Database.SSLMode", cfg.Database.SSLMode, "disable"},
		{"Database.MaxIdleConns", cfg.Database.MaxIdleConns, 10},
		{"Database.MaxOpenConns", cfg.Database.MaxOpenConns, 100},
		{"Database.MaxConnLifetimeSeconds", cfg.Database.MaxConnLifetimeSeconds, 3600},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_USERNAME", "otc_user")
	t.Setenv("DB_PASSWORD", "otc_password")
	t.Setenv("DB_NAME", "otc_db")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("DB_MAX_IDLE_CONNS", "5")
	t.Setenv("DB_MAX_OPEN_CONNS", "50")
	t.Setenv("DB_MAX_CONN_LIFETIME_SECONDS", "1800")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"Database.Host", cfg.Database.Host, "db.example.com"},
		{"Database.Port", cfg.Database.Port, 6543},
		{"Database.Username", cfg.Database.Username, "otc_user"},
		{"Database.Password", cfg.Database.Password, "otc_password"},
		{"Database.Name", cfg.Database.Name, "otc_db"},
		{"Database.SSLMode", cfg.Database.SSLMode, "require"},
		{"Database.MaxIdleConns", cfg.Database.MaxIdleConns, 5},
		{"Database.MaxOpenConns", cfg.Database.MaxOpenConns, 50},
		{"Database.MaxConnLifetimeSeconds", cfg.Database.MaxConnLifetimeSeconds, 1800},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoad_DatabaseValidationError(t *testing.T) {
	// DB_PASSWORD not set (no default) → database.Validate returns error
	t.Setenv("DB_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DB_PASSWORD, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error mentioning 'database', got: %v", err)
	}
}
