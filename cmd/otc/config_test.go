package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	return path
}

// loadYAML writes body as the config file, points CONFIG_PATH at it and runs Load.
func loadYAML(t *testing.T, body string) (*Config, error) {
	t.Helper()
	t.Setenv("CONFIG_PATH", writeConfigFile(t, body))
	return Load()
}

// --- Load ---

// There are no built-in defaults: a db section that sets only the password
// is refused rather than completed with a host, user or sslMode of otc's own.
func TestLoad_NoDefaults(t *testing.T) {
	_, err := loadYAML(t, "db:\n  driver: postgres\n  postgres:\n    password: testpassword\n")
	if err == nil || !strings.Contains(err.Error(), "invalid database configuration") {
		t.Fatalf("expected an incomplete db section to be refused, got: %v", err)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("OTC_TEST_DB_PASSWORD", "otc_password")
	cfg, err := loadYAML(t, `
db:
  driver: postgres
  postgres:
    host: db.example.com
    port: 6543
    user: otc_user
    password: "{{env:OTC_TEST_DB_PASSWORD}}"
    name: otc_db
    sslMode: disable
    pool:
      maxIdleConns: 5
      maxOpenConns: 50
      maxConnLifetimeSeconds: 1800
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"Database.Host", cfg.Database.Postgres.Host, "db.example.com"},
		{"Database.Port", cfg.Database.Postgres.Port, 6543},
		{"Database.Username", cfg.Database.Postgres.User, "otc_user"},
		{"Database.Password", cfg.Database.Postgres.Password, "otc_password"},
		{"Database.Name", cfg.Database.Postgres.Name, "otc_db"},
		{"Database.SSLMode", cfg.Database.Postgres.SSLMode, "disable"},
		{"Database.MaxIdleConns", cfg.Database.Postgres.Pool.MaxIdleConns, 5},
		{"Database.MaxOpenConns", cfg.Database.Postgres.Pool.MaxOpenConns, 50},
		{"Database.MaxConnLifetimeSeconds", cfg.Database.Postgres.Pool.MaxConnLifetimeSeconds, 1800},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// otc runs on the server's own config.yaml: the sections it does not use are
// not decoded, so neither their shape nor their validity concerns it.
func TestLoad_ServerConfigFile(t *testing.T) {
	cfg, err := loadYAML(t, `
server:
  port: 9090
cors:
  allowedOrigins: []
storage:
  type: not-a-backend
db:
  driver: postgres
  postgres:
    host: db
    port: 5432
    user: postgres
    password: testpassword
    name: nsw_db
    sslMode: disable
    pool:
      maxIdleConns: 10
      maxOpenConns: 100
      maxConnLifetimeSeconds: 3600
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Database.Postgres.Host != "db" || cfg.Database.Postgres.Password != "testpassword" {
		t.Errorf("Database = %+v, want the file's db section", cfg.Database)
	}
}

// Placeholders are resolved across the whole file, so one in a section otc
// does not read still has to resolve.
func TestLoad_UnsetPlaceholderElsewhereFails(t *testing.T) {
	_, err := loadYAML(t, `
db:
  postgres:
    password: testpassword
audit:
  apiKey: "{{env:OTC_TEST_UNSET_VAR}}"
`)
	if err == nil || !strings.Contains(err.Error(), "audit.apiKey") {
		t.Fatalf("expected the unset placeholder to fail naming audit.apiKey, got: %v", err)
	}
}

// The server's committed templates load as otc's config too, with the env vars
// .env.example sets for their placeholders.
func TestLoad_ServerTemplates(t *testing.T) {
	for _, k := range []string{"DB_PASSWORD", "ARGUS_API_KEY", "SLPA_WEBHOOK_SECRET", "NOTIFICATION_EMAIL_TOKEN", "NOTIFICATION_SMS_PASSWORD", "STORAGE_LOCAL_PUT_SECRET"} {
		t.Setenv(k, "example-"+k)
	}
	for name, host := range map[string]string{
		"config.example.yaml":        "localhost",
		"config.docker.example.yaml": "db",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CONFIG_PATH", filepath.Join("..", "..", "configs", name))
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.Database.Postgres.Host != host || cfg.Database.Postgres.Password != "example-DB_PASSWORD" {
				t.Errorf("Database = {Host: %q, Password: %q}, want {%q, example-DB_PASSWORD}", cfg.Database.Postgres.Host, cfg.Database.Postgres.Password, host)
			}
		})
	}
}

func TestLoad_DatabaseValidationError(t *testing.T) {
	// No db.postgres.password → database.Validate returns error
	_, err := loadYAML(t, "db:\n  driver: postgres\n  postgres:\n    host: localhost\n")
	if err == nil {
		t.Fatal("expected error for missing db.postgres.password, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error mentioning 'database', got: %v", err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if _, err := Load(); err == nil {
		t.Fatal("expected error for a missing config file, got nil")
	}
}
