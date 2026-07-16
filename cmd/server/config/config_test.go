package config

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/temporal"
)

// validConfig returns a minimal Config that passes Validate().
func validConfig() *Config {
	return &Config{
		Database: database.Config{
			Host:     "localhost",
			Username: "postgres",
			Password: "secret",
			Name:     "testdb",
		},
		Server: ServerConfig{
			ServiceURL: "http://localhost:8080",
		},
		CORS: cors.Config{
			AllowedOrigins: []string{"*"},
		},
		Storage: storage.Config{
			Type:           "local",
			LocalBaseDir:   "./bucket",
			LocalPublicURL: "http://localhost:8080",
			LocalPutSecret: "secret",
			PresignTTL:     15 * time.Minute,
		},
		Authn: authn.Config{
			JWKSURL:   "https://example.com/jwks",
			Issuer:    "https://example.com",
			Audience:  "myapp",
			ClientIDs: []string{"client1"},
		},
		Notification: notification.Config{
			Path: "configs/notification.json",
		},
		Temporal: temporal.Config{
			Host:      "localhost",
			Port:      7233,
			Namespace: "default",
		},
		ArtifactLoader: loaders.Config{
			Type:  loaders.TypeLocal,
			Local: local.Config{Root: "."},
		},
	}
}

// --- HTTPURL ---

func TestHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
		errMsg  string
	}{
		{"valid http", "http://example.com", false, ""},
		{"valid https", "https://example.com/path?q=1", false, ""},
		{"empty string", "", true, "must be a valid absolute URL"},
		{"no scheme", "example.com/path", true, "must be a valid absolute URL"},
		{"ftp scheme", "ftp://example.com", true, "must use http or https"},
		{"scheme only no host", "http://", true, "must be a valid absolute URL"},
		{"path only", "/foo/bar", true, "must be a valid absolute URL"},
		{"whitespace url", "   ", true, "must be a valid absolute URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := HTTPURL("FIELD", tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errMsg != "" && !containsString(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

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

// --- getBoolOrDefault ---

func TestGetBoolOrDefault(t *testing.T) {
	t.Run("unset key returns default false", func(t *testing.T) {
		got := getBoolOrDefault("__UNSET_BOOL__", false)
		if got != false {
			t.Errorf("got %v, want false", got)
		}
	})
	t.Run("unset key returns default true", func(t *testing.T) {
		got := getBoolOrDefault("__UNSET_BOOL2__", true)
		if got != true {
			t.Errorf("got %v, want true", got)
		}
	})
	t.Run("'true' string", func(t *testing.T) {
		t.Setenv("__TEST_BOOL__", "true")
		if !getBoolOrDefault("__TEST_BOOL__", false) {
			t.Error("expected true")
		}
	})
	t.Run("'false' string", func(t *testing.T) {
		t.Setenv("__TEST_BOOL__", "false")
		if getBoolOrDefault("__TEST_BOOL__", true) {
			t.Error("expected false")
		}
	})
	t.Run("'1' string", func(t *testing.T) {
		t.Setenv("__TEST_BOOL__", "1")
		if !getBoolOrDefault("__TEST_BOOL__", false) {
			t.Error("expected true")
		}
	})
	t.Run("'0' string", func(t *testing.T) {
		t.Setenv("__TEST_BOOL__", "0")
		if getBoolOrDefault("__TEST_BOOL__", true) {
			t.Error("expected false")
		}
	})
	t.Run("invalid string returns default", func(t *testing.T) {
		t.Setenv("__TEST_BOOL_BAD__", "yes-please")
		got := getBoolOrDefault("__TEST_BOOL_BAD__", true)
		if !got {
			t.Error("expected default true")
		}
	})
	t.Run("whitespace-only returns default", func(t *testing.T) {
		t.Setenv("__TEST_BOOL_WS__", "  ")
		got := getBoolOrDefault("__TEST_BOOL_WS__", true)
		if !got {
			t.Error("expected default true")
		}
	})
}

// --- getDurationOrDefault ---

func TestGetDurationOrDefault(t *testing.T) {
	t.Run("unset key returns default", func(t *testing.T) {
		got := getDurationOrDefault("__UNSET_DUR__", 5*time.Minute)
		if got != 5*time.Minute {
			t.Errorf("got %v, want 5m", got)
		}
	})
	t.Run("valid duration", func(t *testing.T) {
		t.Setenv("__TEST_DUR__", "30m")
		got := getDurationOrDefault("__TEST_DUR__", 5*time.Minute)
		if got != 30*time.Minute {
			t.Errorf("got %v, want 30m", got)
		}
	})
	t.Run("invalid string returns default", func(t *testing.T) {
		t.Setenv("__TEST_DUR_BAD__", "not-a-duration")
		got := getDurationOrDefault("__TEST_DUR_BAD__", 5*time.Minute)
		if got != 5*time.Minute {
			t.Errorf("got %v, want 5m", got)
		}
	})
	t.Run("whitespace-only returns default", func(t *testing.T) {
		t.Setenv("__TEST_DUR_WS__", "   ")
		got := getDurationOrDefault("__TEST_DUR_WS__", 5*time.Minute)
		if got != 5*time.Minute {
			t.Errorf("got %v, want 5m", got)
		}
	})
}

// --- parseCommaSeparated ---

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
		{"  ,  ,  ", []string{}},
	}
	for _, tc := range tests {
		got := parseCommaSeparated(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("input %q: got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("input %q index %d: got %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// --- parseLogLevel ---

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"verbose", slog.LevelInfo},
	}
	for _, tc := range tests {
		got := parseLogLevel(tc.input)
		if got != tc.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// --- Load ---

func TestLoad_Defaults(t *testing.T) {
	// DB_PASSWORD has no default and is required — set it explicitly.
	// All other env vars are cleared so defaults apply.
	envsToClear := []string{
		"SERVER_PORT", "SERVICE_URL", "DB_HOST", "DB_PORT", "DB_USERNAME",
		"DB_NAME", "DB_SSLMODE", "DB_MAX_IDLE_CONNS", "DB_MAX_OPEN_CONNS",
		"DB_MAX_CONN_LIFETIME_SECONDS", "SERVICES_CONFIG_PATH",
		"PAYMENT_METHODS_CONFIG_PATH", "SERVER_DEBUG", "SERVER_LOG_LEVEL",
		"CORS_ALLOWED_ORIGINS", "CORS_ALLOWED_METHODS", "CORS_ALLOWED_HEADERS",
		"CORS_ALLOW_CREDENTIALS", "CORS_MAX_AGE", "STORAGE_TYPE",
		"STORAGE_LOCAL_BASE_DIR", "STORAGE_LOCAL_PUBLIC_URL", "STORAGE_S3_ENDPOINT",
		"STORAGE_S3_BUCKET", "STORAGE_S3_REGION", "STORAGE_S3_ACCESS_KEY",
		"STORAGE_S3_SECRET_KEY", "STORAGE_S3_USE_SSL", "STORAGE_S3_PUBLIC_URL",
		"STORAGE_LOCAL_PUT_SECRET", "STORAGE_PRESIGN_TTL", "AUTH_JWKS_URL",
		"AUTH_ISSUER", "AUTH_AUDIENCE", "AUTH_CLIENT_IDS",
		"AUTH_JWKS_INSECURE_SKIP_VERIFY", "NOTIFICATIONS_CONFIG_PATH",
		"TEMPORAL_HOST", "TEMPORAL_PORT", "TEMPORAL_NAMESPACE",
	}
	for _, k := range envsToClear {
		t.Setenv(k, "")
	}
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ServiceURL != "http://localhost:8080" {
		t.Errorf("Server.ServiceURL = %q, want http://localhost:8080", cfg.Server.ServiceURL)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host = %q, want localhost", cfg.Database.Host)
	}
	if cfg.Database.Password != "testpassword" {
		t.Errorf("Database.Password not propagated")
	}
	if cfg.Temporal.Namespace != "default" {
		t.Errorf("Temporal.Namespace = %q, want default", cfg.Temporal.Namespace)
	}
}

func TestLoad_CustomPort(t *testing.T) {
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("SERVICE_URL", "")
	t.Setenv("STORAGE_LOCAL_PUBLIC_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.ServiceURL != "http://localhost:9090" {
		t.Errorf("Server.ServiceURL = %q, want http://localhost:9090", cfg.Server.ServiceURL)
	}
	if cfg.Storage.LocalPublicURL != "http://localhost:9090" {
		t.Errorf("Storage.LocalPublicURL = %q, want http://localhost:9090", cfg.Storage.LocalPublicURL)
	}
}

func TestLoad_CustomServiceURL(t *testing.T) {
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")
	t.Setenv("SERVICE_URL", "https://api.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.ServiceURL != "https://api.example.com" {
		t.Errorf("Server.ServiceURL = %q, want https://api.example.com", cfg.Server.ServiceURL)
	}
}

func TestLoad_CustomLogLevel(t *testing.T) {
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("ARTIFACT_LOCAL_ROOT", ".")
	t.Setenv("SERVER_LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want Debug", cfg.Server.LogLevel)
	}
}

func TestLoad_InvalidServiceURL(t *testing.T) {
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("SERVICE_URL", "not-a-url")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid SERVICE_URL, got nil")
	}
}

func TestLoad_DatabaseValidationError(t *testing.T) {
	// DB_PASSWORD not set (no default) → database.Validate returns error
	t.Setenv("DB_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DB_PASSWORD, got nil")
	}
	if !containsString(err.Error(), "database") {
		t.Errorf("expected error mentioning 'database', got: %v", err)
	}
}

// --- Config.Validate ---

func TestConfigValidate_Success(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestConfigValidate_EmptyServiceURL(t *testing.T) {
	cfg := validConfig()
	cfg.Server.ServiceURL = ""
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "SERVICE_URL is required") {
		t.Errorf("expected SERVICE_URL required error, got: %v", err)
	}
}

func TestConfigValidate_InvalidServiceURL(t *testing.T) {
	cfg := validConfig()
	cfg.Server.ServiceURL = "not-a-url"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid SERVICE_URL")
	}
}

func TestConfigValidate_ServiceURLWrongScheme(t *testing.T) {
	cfg := validConfig()
	cfg.Server.ServiceURL = "ftp://example.com"
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "must use http or https") {
		t.Errorf("expected scheme error, got: %v", err)
	}
}

func TestConfigValidate_DatabaseError(t *testing.T) {
	cfg := validConfig()
	cfg.Database = database.Config{} // all fields empty → DB_HOST required
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid database configuration") {
		t.Errorf("expected database config error, got: %v", err)
	}
}

func TestConfigValidate_StorageError(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = storage.Config{Type: "local"} // missing LocalBaseDir
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid storage configuration") {
		t.Errorf("expected storage config error, got: %v", err)
	}
}

func TestConfigValidate_ArtifactLoaderError(t *testing.T) {
	cfg := validConfig()
	cfg.ArtifactLoader = loaders.Config{Type: "bogus"} // unsupported type
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid artifact loader configuration") {
		t.Errorf("expected artifact loader config error, got: %v", err)
	}
}

func TestConfigValidate_AuthnError(t *testing.T) {
	cfg := validConfig()
	cfg.Authn = authn.Config{} // all empty → JWKSURL required
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid authn configuration") {
		t.Errorf("expected authn config error, got: %v", err)
	}
}

func TestConfigValidate_TemporalError(t *testing.T) {
	cfg := validConfig()
	cfg.Temporal = temporal.Config{Host: "localhost", Port: 0, Namespace: "default"} // port 0 → invalid
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid temporal configuration") {
		t.Errorf("expected temporal config error, got: %v", err)
	}
}

func TestConfigValidate_CORSError(t *testing.T) {
	cfg := validConfig()
	cfg.CORS = cors.Config{} // empty AllowedOrigins → CORS error
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid CORS configuration") {
		t.Errorf("expected CORS config error, got: %v", err)
	}
}

func TestConfigValidate_NotificationError(t *testing.T) {
	cfg := validConfig()
	cfg.Notification = notification.Config{} // empty Path → error
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected notification config error")
	}
	if !errors.Is(err, notification.ErrConfigPathRequired) && !containsString(err.Error(), "invalid notification configuration") {
		t.Errorf("expected notification config error, got: %v", err)
	}
}

// containsString is a helper to check if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
