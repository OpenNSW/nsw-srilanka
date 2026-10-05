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

func TestLoadConfigFile_Empty(t *testing.T) {
	fc, err := loadConfigFile(writeConfigFile(t, ""))
	if err != nil {
		t.Fatalf("expected an empty file to be valid, got: %v", err)
	}
	if len(fc.RefID.Issuers) != 0 {
		t.Errorf("expected no refid issuers, got %d", len(fc.RefID.Issuers))
	}
}

func TestLoadConfigFile_RefIDOmitted(t *testing.T) {
	fc, err := loadConfigFile(writeConfigFile(t, "refid:\n  issuers: []\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.RefID.Issuers) != 0 {
		t.Errorf("expected no refid issuers, got %d", len(fc.RefID.Issuers))
	}
}

func TestLoadConfigFile_RefIDDecoded(t *testing.T) {
	fc, err := loadConfigFile(writeConfigFile(t, `
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

func TestLoadConfigFile_ResolvesPlaceholders(t *testing.T) {
	t.Setenv("REFID_TEST_ISSUER", "TNSW")
	fc, err := loadConfigFile(writeConfigFile(t, `
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

func TestLoadConfigFile_UnsetPlaceholderFails(t *testing.T) {
	_, err := loadConfigFile(writeConfigFile(t, `
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

func TestLoadConfigFile_Malformed(t *testing.T) {
	if _, err := loadConfigFile(writeConfigFile(t, "refid: [unclosed\n")); err == nil {
		t.Fatal("expected malformed YAML to be rejected, got nil")
	}
}

func TestLoad_ConfigFileMissing(t *testing.T) {
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("SLPA_WEBHOOK_SECRET", "a-secret-shared-with-slpa")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
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
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("SLPA_WEBHOOK_SECRET", "a-secret-shared-with-slpa")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	t.Setenv("CONFIG_PATH", writeConfigFile(t, `
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

// The committed template must always load: `make setup` seeds the live file
// from it, and the e2e harness points CONFIG_PATH straight at it.
func TestLoadConfigFile_ExampleLoads(t *testing.T) {
	if _, err := loadConfigFile(filepath.Join("..", "..", "..", "configs", "config.example.yaml")); err != nil {
		t.Fatalf("configs/config.example.yaml does not load: %v", err)
	}
}

func TestLoad_ConfigFileMode(t *testing.T) {
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("SLPA_WEBHOOK_SECRET", "a-secret-shared-with-slpa")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")

	for name, tc := range map[string]struct {
		body    string
		want    Mode
		wantErr bool
	}{
		"omitted defaults to tnsw": {"", ModeTNSW, false},
		"tnsw":                     {"mode: tnsw\n", ModeTNSW, false},
		"agency":                   {"mode: agency\n", ModeAgency, false},
		"unknown is rejected":      {"mode: both\n", "", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CONFIG_PATH", writeConfigFile(t, tc.body))
			cfg, err := Load()
			if tc.wantErr {
				if err == nil || !containsString(err.Error(), "invalid mode") {
					t.Fatalf("expected an invalid mode error, got: %v", err)
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
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("SLPA_WEBHOOK_SECRET", "")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")

	t.Setenv("CONFIG_PATH", writeConfigFile(t, "mode: agency\n"))
	if _, err := Load(); err != nil {
		t.Fatalf("agency: Load() unexpected error: %v", err)
	}

	t.Setenv("CONFIG_PATH", writeConfigFile(t, "mode: tnsw\n"))
	if _, err := Load(); err == nil || !containsString(err.Error(), "SLPA_WEBHOOK_SECRET") {
		t.Fatalf("tnsw: expected the SLPA secret to be required, got: %v", err)
	}
}
