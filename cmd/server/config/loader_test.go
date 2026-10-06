package config

import (
	"os"
	"path/filepath"
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

func TestLoadFile_Empty(t *testing.T) {
	fc, err := loadFile(writeConfigFile(t, ""))
	if err != nil {
		t.Fatalf("expected an empty file to be valid, got: %v", err)
	}
	if len(fc.RefID.Issuers) != 0 {
		t.Errorf("expected no refid issuers, got %d", len(fc.RefID.Issuers))
	}
}

func TestLoadFile_RefIDOmitted(t *testing.T) {
	fc, err := loadFile(writeConfigFile(t, "refid:\n  issuers: []\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.RefID.Issuers) != 0 {
		t.Errorf("expected no refid issuers, got %d", len(fc.RefID.Issuers))
	}
}

func TestLoadFile_RefIDDecoded(t *testing.T) {
	fc, err := loadFile(writeConfigFile(t, `
refid:
  issuers:
    - issuer: TNSW
      formats:
        - idType: consignment_ref
          segments:
            - type: literal
              value: "TNSW-"
            - type: list
              list: port
              param: portCode
            - type: sequence
              sequence:
                scopeKey: "{issuer}:{idType}:{portCode}"
                padding: 6
        - idType: voucher_code
          segments:
            - type: random
              random:
                scopeKey: "{issuer}:{idType}"
                charset: alphanumeric
                length: 8
  lists:
    port: [CMB, HBT]
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fc.RefID.Issuers) != 1 || fc.RefID.Issuers[0].Issuer != "TNSW" {
		t.Fatalf("expected one TNSW issuer, got %+v", fc.RefID.Issuers)
	}
	formats := fc.RefID.Issuers[0].Formats
	if len(formats) != 2 {
		t.Fatalf("expected 2 formats, got %d", len(formats))
	}
	seq := formats[0].Segments[2].Sequence
	if seq == nil || seq.ScopeKey != "{issuer}:{idType}:{portCode}" || seq.Padding != 6 {
		t.Errorf("sequence segment not decoded: %+v", seq)
	}
	rnd := formats[1].Segments[0].Random
	if rnd == nil || rnd.Charset != "alphanumeric" || rnd.Length != 8 {
		t.Errorf("random segment not decoded: %+v", rnd)
	}
	if got := fc.RefID.Lists["port"]; len(got) != 2 || got[0] != "CMB" {
		t.Errorf("lists not decoded: %+v", fc.RefID.Lists)
	}
}

func TestLoadFile_ResolvesPlaceholders(t *testing.T) {
	t.Setenv("REFID_TEST_ISSUER", "TNSW")
	fc, err := loadFile(writeConfigFile(t, `
refid:
  issuers:
    - issuer: "{{env:REFID_TEST_ISSUER}}"
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.RefID.Issuers) != 1 || fc.RefID.Issuers[0].Issuer != "TNSW" {
		t.Errorf("expected the issuer to resolve from the env var, got %+v", fc.RefID.Issuers)
	}
}

func TestLoadFile_UnsetPlaceholderFails(t *testing.T) {
	_, err := loadFile(writeConfigFile(t, `
refid:
  issuers:
    - issuer: "{{env:REFID_TEST_UNSET_VAR}}"
`))
	if err == nil {
		t.Fatal("expected an unset placeholder to fail, got nil")
	}
	if !containsString(err.Error(), "refid.issuers[0].issuer") {
		t.Errorf("expected the error to name the offending key, got: %v", err)
	}
}

func TestLoadFile_Malformed(t *testing.T) {
	if _, err := loadFile(writeConfigFile(t, "refid: [unclosed\n")); err == nil {
		t.Fatal("expected malformed YAML to be rejected, got nil")
	}
}

func TestLoad_ConfigFileMissing(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "does-not-exist.yaml"))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for a missing config file, got nil")
	}
	if !containsString(err.Error(), "config file") {
		t.Errorf("expected error mentioning 'config file', got: %v", err)
	}
}

func TestLoad_ConfigFileRefID(t *testing.T) {
	t.Setenv("CONFIG_PATH", writeConfigFile(t, baseYAML+`
refid:
  issuers:
    - issuer: TNSW
      formats:
        - idType: consignment_ref
          segments:
            - type: literal
              value: "TNSW"
`))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if len(cfg.RefID.Issuers) != 1 || cfg.RefID.Issuers[0].Formats[0].IDType != "consignment_ref" {
		t.Errorf("expected the refid section to reach Config, got %+v", cfg.RefID)
	}
}

// The committed templates must always load and validate: `make setup` seeds
// the live files from them, compose mounts the docker one, and the e2e harness
// points CONFIG_PATH straight at config.example.yaml. With no built-in
// defaults, each has to set every required setting itself. Their placeholders
// resolve from the env vars .env.example sets.
func TestLoadFile_ExamplesLoad(t *testing.T) {
	for _, k := range []string{"DB_PASSWORD", "ARGUS_API_KEY", "SLPA_WEBHOOK_SECRET", "NOTIFICATION_EMAIL_TOKEN", "NOTIFICATION_SMS_PASSWORD", "STORAGE_LOCAL_PUT_SECRET"} {
		t.Setenv(k, "example-"+k)
	}
	// Both are dev templates, run under APP_ENV=development (make dev, make
	// test-e2e), which lets their insecure-TLS settings through.
	t.Setenv("APP_ENV", "development")
	for _, name := range []string{"config.example.yaml", "config.docker.example.yaml"} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadFile(filepath.Join("..", "..", "..", "configs", name))
			if err != nil {
				t.Fatalf("configs/%s does not load: %v", name, err)
			}
			if cfg.Database.Postgres.Password != "example-DB_PASSWORD" {
				t.Errorf("db.postgres.password = %q, want it resolved from DB_PASSWORD", cfg.Database.Postgres.Password)
			}
			if got := cfg.Notification.Providers.SMS.Password; got != "example-NOTIFICATION_SMS_PASSWORD" {
				t.Errorf("notification.providers.sms.password = %q, want it resolved from NOTIFICATION_SMS_PASSWORD", got)
			}
			if err := cfg.Validate(); err != nil {
				t.Errorf("configs/%s does not validate: %v", name, err)
			}
		})
	}
}

func TestLoad_ConfigFileMode(t *testing.T) {
	for name, tc := range map[string]struct {
		body    string
		want    Mode
		wantErr string
	}{
		"omitted is rejected": {configYAML(t, "", "mode"), "", "mode is required"},
		"tnsw":                {configYAML(t, "mode: tnsw\n"), ModeTNSW, ""},
		"agency":              {configYAML(t, "mode: agency\n"), ModeAgency, ""},
		"unknown is rejected": {configYAML(t, "mode: both\n"), "", "invalid mode"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadYAML(t, tc.body)
			if tc.wantErr != "" {
				if err == nil || !containsString(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.Mode != tc.want {
				t.Errorf("Mode = %q, want %q", cfg.Mode, tc.want)
			}
		})
	}
}

// An agency builds none of TNSW's integrations, so it starts without their secrets.
func TestLoad_AgencyModeNeedsNoSLPASecret(t *testing.T) {
	const secret = "integrations.slpaWebhookSecret"

	if _, err := loadYAML(t, configYAML(t, "mode: agency\n", secret)); err != nil {
		t.Fatalf("agency: Load() unexpected error: %v", err)
	}

	if _, err := loadYAML(t, configYAML(t, "mode: tnsw\n", secret)); err == nil || !containsString(err.Error(), "slpa webhook") {
		t.Fatalf("tnsw: expected the SLPA secret to be required, got: %v", err)
	}
}
