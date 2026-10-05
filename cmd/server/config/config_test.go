package config

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"

	"github.com/OpenNSW/core/temporal"
	integrations "github.com/OpenNSW/nsw-srilanka/external-integration"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// The smallest config.yaml Load accepts, in pieces so a test can swap one
// section for its own (yaml rejects a key defined twice): the settings with no
// default — the database password, CORS origins, notification providers and
// the SLPA webhook secret — plus an artifact root that exists.
const (
	databaseYAML = `
db:
  postgres:
    password: testpassword
`
	corsYAML = `
cors:
  allowedOrigins: ["http://localhost:3000"]
`
	restYAML = `
notification:
  providers:
    email:
      baseURL: https://email.example.com
integrations:
  slpaWebhookSecret: a-secret-shared-with-slpa
artifactLoader:
  local:
    root: "."
`
	baseYAML = databaseYAML + corsYAML + restYAML
)

// loadYAML writes body as config.yaml, points CONFIG_PATH at it and runs Load.
func loadYAML(t *testing.T, body string) (*Config, error) {
	t.Helper()
	t.Setenv("CONFIG_PATH", writeConfigFile(t, body))
	return Load()
}

// validConfig returns a minimal Config that passes Validate().
func validConfig() *Config {
	return &Config{
		Database: database.Config{
			Driver: database.Postgres,
			Postgres: &database.PostgresConfig{
				Host:     "localhost",
				User:     "postgres",
				Password: "secret",
				Name:     "testdb",
			},
		},
		Server: ServerConfig{
			ServiceURL:        "http://localhost:8080",
			MaxRequestBytes:   33554432,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		CORS: cors.Config{
			AllowedOrigins: []string{"http://localhost:3000"},
		},
		Storage: nswstorage.Config{Config: storage.Config{
			Type: storage.TypeLocal,
			Local: drivers.LocalConfig{
				BaseDir:   "./bucket",
				PublicURL: "http://localhost:8080",
				PutSecret: "secret",
			},
			PresignTTLSeconds: 900,
		}},
		Integrations: integrations.Config{
			SLPAWebhookSecret: "a-secret-shared-with-slpa",
		},
		Authn: authn.Config{
			JWKSURL:   "https://example.com/jwks",
			Issuer:    "https://example.com",
			Audience:  "myapp",
			ClientIDs: []string{"client1"},
		},
		Notification: notification.Config{
			Providers: map[notification.ChannelType]map[string]any{
				"email": {"baseURL": "https://email.example.com"},
			},
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

// --- ServerConfig.Validate ---

func TestServerConfigValidate_NonPositiveLimits(t *testing.T) {
	base := func() ServerConfig {
		return validConfig().Server
	}
	tests := []struct {
		name   string
		mutate func(*ServerConfig)
		errMsg string
	}{
		{"zero MaxRequestBytes", func(s *ServerConfig) { s.MaxRequestBytes = 0 }, "server.maxRequestBytes must be greater than zero"},
		{"negative MaxRequestBytes", func(s *ServerConfig) { s.MaxRequestBytes = -1 }, "server.maxRequestBytes must be greater than zero"},
		{"zero ReadHeaderTimeout", func(s *ServerConfig) { s.ReadHeaderTimeout = 0 }, "server.readHeaderTimeout must be greater than zero"},
		{"negative ReadHeaderTimeout", func(s *ServerConfig) { s.ReadHeaderTimeout = -1 * time.Second }, "server.readHeaderTimeout must be greater than zero"},
		{"zero ReadTimeout", func(s *ServerConfig) { s.ReadTimeout = 0 }, "server.readTimeout must be greater than zero"},
		{"negative ReadTimeout", func(s *ServerConfig) { s.ReadTimeout = -1 * time.Second }, "server.readTimeout must be greater than zero"},
		{"zero WriteTimeout", func(s *ServerConfig) { s.WriteTimeout = 0 }, "server.writeTimeout must be greater than zero"},
		{"negative WriteTimeout", func(s *ServerConfig) { s.WriteTimeout = -1 * time.Second }, "server.writeTimeout must be greater than zero"},
		{"zero IdleTimeout", func(s *ServerConfig) { s.IdleTimeout = 0 }, "server.idleTimeout must be greater than zero"},
		{"negative IdleTimeout", func(s *ServerConfig) { s.IdleTimeout = -1 * time.Second }, "server.idleTimeout must be greater than zero"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := base()
			tc.mutate(&s)
			err := s.Validate()
			if err == nil || !containsString(err.Error(), tc.errMsg) {
				t.Errorf("expected error containing %q, got %v", tc.errMsg, err)
			}
		})
	}
}

// --- Load ---

func TestLoad_Defaults(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"Server.Port", cfg.Server.Port, 8080},
		{"Server.ServiceURL", cfg.Server.ServiceURL, "http://localhost:8080"},
		{"Server.MaxRequestBytes", cfg.Server.MaxRequestBytes, int64(33554432)},
		{"Server.ReadHeaderTimeout", cfg.Server.ReadHeaderTimeout, 5 * time.Second},
		{"Server.ReadTimeout", cfg.Server.ReadTimeout, 15 * time.Second},
		{"Server.WriteTimeout", cfg.Server.WriteTimeout, 30 * time.Second},
		{"Server.IdleTimeout", cfg.Server.IdleTimeout, 60 * time.Second},
		{"Server.ServicesConfigPath", cfg.Server.ServicesConfigPath, "configs/services.json"},
		{"Server.CatalogConfigPath", cfg.Server.CatalogConfigPath, "configs/catalog.json"},
		{"Server.LogLevel", cfg.Server.LogLevel, slog.LevelInfo},
		{"Database.Host", cfg.Database.Postgres.Host, "localhost"},
		{"Database.Port", cfg.Database.Postgres.Port, 5432},
		{"Database.Password", cfg.Database.Postgres.Password, "testpassword"},
		{"Database.SSLMode", cfg.Database.Postgres.SSLMode, "require"},
		{"Temporal.Namespace", cfg.Temporal.Namespace, "default"},
		{"CORS.AllowCredentials", cfg.CORS.AllowCredentials, true},
		{"CORS.MaxAge", cfg.CORS.MaxAge, 3600},
		{"Storage.Type", cfg.Storage.Type, "local"},
		{"Storage.Local.BaseDir", cfg.Storage.Local.BaseDir, "./bucket"},
		{"Storage.Local.PublicURL", cfg.Storage.Local.PublicURL, "http://localhost:8080"},
		{"Storage.PresignTTLSeconds", cfg.Storage.PresignTTLSeconds, 900},
		{"Authn.Issuer", cfg.Authn.Issuer, "https://localhost:8090"},
		{"ArtifactLoader.Type", cfg.ArtifactLoader.Type, loaders.TypeLocal},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	if len(cfg.CORS.AllowedOrigins) != 1 || cfg.CORS.AllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("CORS.AllowedOrigins = %v, want [http://localhost:3000]", cfg.CORS.AllowedOrigins)
	}
	if len(cfg.CORS.AllowedMethods) != 5 {
		t.Errorf("CORS.AllowedMethods = %v, want the 5 default methods", cfg.CORS.AllowedMethods)
	}
}

func TestLoad_DefaultFailClosed(t *testing.T) {
	_, err := loadYAML(t, databaseYAML+restYAML)
	if err == nil {
		t.Fatal("expected error when cors.allowedOrigins is not set, got nil")
	}
	if !containsString(err.Error(), "invalid CORS configuration") {
		t.Errorf("expected a CORS configuration error, got: %v", err)
	}
}

func TestLoad_InvalidCORSWildcardWithCredentials(t *testing.T) {
	_, err := loadYAML(t, databaseYAML+restYAML+`
cors:
  allowedOrigins: ["*"]
  allowCredentials: true
`)
	if err == nil {
		t.Fatal("expected error for wildcard origin with credentials=true, got nil")
	}
	if !containsString(err.Error(), "wildcard origin '*' is not allowed when AllowCredentials is true") {
		t.Errorf("expected error mentioning wildcard credentials constraint, got: %v", err)
	}
}

func TestLoad_CustomPort(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML+`
server:
  port: 9090
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.ServiceURL != "http://localhost:9090" {
		t.Errorf("Server.ServiceURL = %q, want http://localhost:9090", cfg.Server.ServiceURL)
	}
	if cfg.Storage.Local.PublicURL != "http://localhost:9090" {
		t.Errorf("Storage.Local.PublicURL = %q, want http://localhost:9090", cfg.Storage.Local.PublicURL)
	}
}

func TestLoad_CustomServiceURL(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML+`
server:
  serviceURL: https://api.example.com
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.ServiceURL != "https://api.example.com" {
		t.Errorf("Server.ServiceURL = %q, want https://api.example.com", cfg.Server.ServiceURL)
	}
	if cfg.Storage.Local.PublicURL != "https://api.example.com" {
		t.Errorf("Storage.Local.PublicURL = %q, want it to follow server.serviceURL", cfg.Storage.Local.PublicURL)
	}
}

func TestLoad_CustomLogLevel(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML+`
server:
  logLevel: debug
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want Debug", cfg.Server.LogLevel)
	}
}

func TestLoad_CustomServerLimits(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML+`
server:
  maxRequestBytes: 262144
  readHeaderTimeout: 2s
  readTimeout: 7s
  writeTimeout: 9s
  idleTimeout: 11s
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.MaxRequestBytes != 262144 {
		t.Errorf("Server.MaxRequestBytes = %d, want 262144", cfg.Server.MaxRequestBytes)
	}
	if cfg.Server.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("Server.ReadHeaderTimeout = %v, want 2s", cfg.Server.ReadHeaderTimeout)
	}
	if cfg.Server.ReadTimeout != 7*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want 7s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 9*time.Second {
		t.Errorf("Server.WriteTimeout = %v, want 9s", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 11*time.Second {
		t.Errorf("Server.IdleTimeout = %v, want 11s", cfg.Server.IdleTimeout)
	}
}

// Unlike the env vars this file replaced, a value that does not parse fails
// the load rather than silently falling back to the default.
func TestLoad_UnparseableValueRejected(t *testing.T) {
	for name, server := range map[string]string{
		"duration":  "readTimeout: soon",
		"log level": "logLevel: loud",
		"int":       "port: eighty",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadYAML(t, baseYAML+"\nserver:\n  "+server+"\n"); err == nil {
				t.Fatalf("expected %q to be rejected, got nil", server)
			}
		})
	}
}

func TestLoad_InvalidServiceURL(t *testing.T) {
	_, err := loadYAML(t, baseYAML+`
server:
  serviceURL: not-a-url
`)
	if err == nil {
		t.Fatal("expected error for invalid server.serviceURL, got nil")
	}
}

func TestLoad_ZeroReadTimeoutRejected(t *testing.T) {
	_, err := loadYAML(t, baseYAML+`
server:
  readTimeout: 0s
`)
	if err == nil || !containsString(err.Error(), "server.readTimeout must be greater than zero") {
		t.Fatalf("expected server.readTimeout validation error, got: %v", err)
	}
}

func TestLoad_DatabaseValidationError(t *testing.T) {
	// db.postgres.password has no default → database.Validate returns error
	_, err := loadYAML(t, corsYAML+restYAML)
	if err == nil {
		t.Fatal("expected error for missing db.postgres.password, got nil")
	}
	if !containsString(err.Error(), "database") {
		t.Errorf("expected error mentioning 'database', got: %v", err)
	}
}

func TestLoad_SecretPlaceholders(t *testing.T) {
	t.Setenv("TEST_DB_PASSWORD", "from-env")
	secret := filepath.Join(t.TempDir(), "argus-key")
	if err := os.WriteFile(secret, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	cfg, err := loadYAML(t, corsYAML+restYAML+`
db:
  postgres:
    password: "{{env:TEST_DB_PASSWORD}}"
audit:
  baseURL: http://argus:3001
  apiKey: "{{file:`+secret+`}}"
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Database.Postgres.Password != "from-env" {
		t.Errorf("Database.Password = %q, want it resolved from the env var", cfg.Database.Postgres.Password)
	}
	if got := cfg.Audit.ClientConfig(); got.BaseURL != "http://argus:3001" || got.APIKey != "from-file" {
		t.Errorf("Audit.ClientConfig() = {BaseURL: %q, APIKey: %q}, want the file's values", got.BaseURL, got.APIKey)
	}
}

func TestLoad_UnsetSecretPlaceholderFails(t *testing.T) {
	_, err := loadYAML(t, corsYAML+restYAML+`
db:
  postgres:
    password: "{{env:TEST_DB_PASSWORD_UNSET}}"
`)
	if err == nil || !containsString(err.Error(), "db.postgres.password") {
		t.Fatalf("expected an unset placeholder to fail naming db.postgres.password, got: %v", err)
	}
}

func TestLoad_NotificationProviders(t *testing.T) {
	cfg, err := loadYAML(t, databaseYAML+corsYAML+`
notification:
  providers:
    email:
      baseURL: https://email.example.com
      token: email-token
    sms:
      baseURL: https://sms.example.com
      sidCode: sid
integrations:
  slpaWebhookSecret: a-secret-shared-with-slpa
artifactLoader:
  local:
    root: "."
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := cfg.Notification.Providers["email"]["token"]; got != "email-token" {
		t.Errorf("email token = %v, want email-token", got)
	}
	if got := cfg.Notification.Providers["sms"]["sidCode"]; got != "sid" {
		t.Errorf("sms sidCode = %v, want sid", got)
	}
}

func TestLoad_NotificationProvidersMissing(t *testing.T) {
	_, err := loadYAML(t, databaseYAML+corsYAML+`
integrations:
  slpaWebhookSecret: a-secret-shared-with-slpa
artifactLoader:
  local:
    root: "."
`)
	if err == nil || !containsString(err.Error(), "notification") {
		t.Fatalf("expected a notification configuration error, got: %v", err)
	}
}

func TestLoad_StorageS3(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML+`
storage:
  type: s3
  s3:
    bucket: uploads
    region: ap-south-1
    endpoint: http://minio:9000
  presignTTLSeconds: 60
`)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Storage.S3.Bucket != "uploads" || cfg.Storage.S3.Region != "ap-south-1" || cfg.Storage.S3.Endpoint != "http://minio:9000" {
		t.Errorf("Storage.S3 = %+v, want the file's values", cfg.Storage.S3)
	}
	if cfg.Storage.PresignTTLSeconds != 60 {
		t.Errorf("Storage.PresignTTLSeconds = %d, want 60", cfg.Storage.PresignTTLSeconds)
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
	if err == nil || !containsString(err.Error(), "server.serviceURL is required") {
		t.Errorf("expected server.serviceURL required error, got: %v", err)
	}
}

func TestConfigValidate_InvalidServiceURL(t *testing.T) {
	cfg := validConfig()
	cfg.Server.ServiceURL = "not-a-url"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid server.serviceURL")
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
	cfg.Database = database.Config{} // no driver → driver required
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid database configuration") {
		t.Errorf("expected database config error, got: %v", err)
	}
}

func TestConfigValidate_StorageError(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = nswstorage.Config{Config: storage.Config{Type: storage.TypeLocal}} // missing local.baseDir
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "invalid storage configuration") {
		t.Errorf("expected storage config error, got: %v", err)
	}
}

func TestConfigValidate_StorageProxy(t *testing.T) {
	cfg := validConfig()
	// Proxy mode validates the proxy settings, not the core/storage backend,
	// which would reject "proxy" as an unknown backend type.
	cfg.Storage = nswstorage.Config{
		Config: storage.Config{Type: nswstorage.TypeProxy},
		Proxy: nswstorage.ProxyConfig{
			Service:      "files-api",
			UploadPath:   nswstorage.DefaultProxyUploadPath,
			DownloadPath: nswstorage.DefaultProxyDownloadPath,
			DeletePath:   nswstorage.DefaultProxyDeletePath,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for a complete proxy configuration", err)
	}

	cfg.Storage.Proxy.Service = ""
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "storage.proxy.service") {
		t.Errorf("expected storage.proxy.service error, got: %v", err)
	}
}

func TestLoad_StorageProxyDefaults(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML+`
storage:
  type: proxy
  proxy:
    service: files-api
`)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := nswstorage.ProxyConfig{
		Service:      "files-api",
		UploadPath:   "/api/v1/storage",
		DownloadPath: "/api/v1/storage/{key}",
		DeletePath:   "/api/v1/storage/{key}",
	}
	if cfg.Storage.Proxy != want {
		t.Errorf("Storage.Proxy = %+v, want %+v", cfg.Storage.Proxy, want)
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
	if err == nil || !containsString(err.Error(), "CORS_ALLOWED_ORIGINS is required") {
		t.Errorf("expected CORS config error mentioning 'CORS_ALLOWED_ORIGINS is required', got: %v", err)
	}
}

func TestConfigValidate_CORSWildcardCredentialsError(t *testing.T) {
	cfg := validConfig()
	cfg.CORS = cors.Config{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	}
	err := cfg.Validate()
	if err == nil || !containsString(err.Error(), "wildcard origin '*' is not allowed when AllowCredentials is true") {
		t.Errorf("expected invalid CORS configuration error for wildcard origin with AllowCredentials=true, got: %v", err)
	}
}

func TestConfigValidate_NotificationError(t *testing.T) {
	cfg := validConfig()
	cfg.Notification = notification.Config{} // no Providers → error
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected notification config error")
	}
	if !errors.Is(err, notification.ErrProvidersRequired) && !containsString(err.Error(), "invalid notification configuration") {
		t.Errorf("expected notification config error, got: %v", err)
	}
}

// --- insecure-TLS guard (APP_ENV) ---

func TestConfigValidate_JWKSInsecure_FailsClosedOutsideDev(t *testing.T) {
	t.Setenv("APP_ENV", "production") // not development
	cfg := validConfig()
	cfg.Authn.InsecureSkipTLSVerify = true
	if err := cfg.Validate(); err == nil || !containsString(err.Error(), "APP_ENV") {
		t.Fatalf("expected APP_ENV guard error for insecure JWKS, got: %v", err)
	}
}

func TestConfigValidate_JWKSInsecure_AllowedInDev(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	cfg := validConfig()
	cfg.Authn.InsecureSkipTLSVerify = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error with insecure JWKS in development, got: %v", err)
	}
}

func TestConfigValidate_ServicesInsecure_FailsClosedOutsideDev(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, []byte(`{"services":[{"id":"npqs","auth":{"type":"oauth2","options":{"insecure_skip_tls_verify":true}}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := validConfig()
	cfg.Server.ServicesConfigPath = path
	if err := cfg.Validate(); err == nil || !containsString(err.Error(), "APP_ENV") {
		t.Fatalf("expected APP_ENV guard error for insecure services entry, got: %v", err)
	}
}

func TestConfigValidate_ServicesInsecure_AllowedInDev(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, []byte(`{"services":[{"id":"npqs","auth":{"type":"oauth2","options":{"insecure_skip_tls_verify":true}}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := validConfig()
	cfg.Server.ServicesConfigPath = path
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error with insecure services entry in development, got: %v", err)
	}
}

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
			t.Setenv("APP_ENV", c.val)
			if got := isDevEnvironment(); got != c.want {
				t.Fatalf("isDevEnvironment() with APP_ENV=%q = %v, want %v", c.val, got, c.want)
			}
		})
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
