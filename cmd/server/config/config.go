package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/github"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/artifact/loaders/s3"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/temporal"

	"github.com/LSFLK/argus/pkg/audit"
)

// Config holds all configuration for the application.
type Config struct {
	Database     database.Config
	Server       ServerConfig
	CORS         cors.Config
	Storage      storage.Config
	Authn        authn.Config
	Notification notification.Config
	Temporal     temporal.Config
	Audit        audit.Config

	ArtifactLoader loaders.Config
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	Port                     int
	ServiceURL               string
	ServicesConfigPath       string
	PaymentMethodsConfigPath string
	TaskAuthzConfigPath      string
	Debug                    bool
	LogLevel                 slog.Level
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	serverPort := getIntEnvOrDefault("SERVER_PORT", 8080)

	cfg := &Config{
		Database: database.Config{
			Host:                   getEnvOrDefault("DB_HOST", "localhost"),
			Port:                   getIntEnvOrDefault("DB_PORT", 5432),
			Username:               getEnvOrDefault("DB_USERNAME", "postgres"),
			Password:               os.Getenv("DB_PASSWORD"), // No default for security
			Name:                   getEnvOrDefault("DB_NAME", "nsw_db"),
			SSLMode:                getEnvOrDefault("DB_SSLMODE", "disable"),
			MaxIdleConns:           getIntEnvOrDefault("DB_MAX_IDLE_CONNS", 10),
			MaxOpenConns:           getIntEnvOrDefault("DB_MAX_OPEN_CONNS", 100),
			MaxConnLifetimeSeconds: getIntEnvOrDefault("DB_MAX_CONN_LIFETIME_SECONDS", 3600),
		},
		Server: ServerConfig{
			Port:                     serverPort,
			ServiceURL:               getEnvOrDefault("SERVICE_URL", fmt.Sprintf("http://localhost:%d", serverPort)),
			ServicesConfigPath:       getEnvOrDefault("SERVICES_CONFIG_PATH", "configs/services.json"),
			PaymentMethodsConfigPath: getEnvOrDefault("PAYMENT_METHODS_CONFIG_PATH", "configs/payment_methods.json"),
			TaskAuthzConfigPath:      getEnvOrDefault("TASK_AUTHZ_CONFIG_PATH", "configs/task_authz.json"),
			Debug:                    getBoolOrDefault("SERVER_DEBUG", true),
			LogLevel:                 parseLogLevel(getEnvOrDefault("SERVER_LOG_LEVEL", "info")),
		},
		CORS: cors.Config{
			AllowedOrigins:   parseCommaSeparated(getEnvOrDefault("CORS_ALLOWED_ORIGINS", "")),
			AllowedMethods:   parseCommaSeparated(getEnvOrDefault("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,OPTIONS")),
			AllowedHeaders:   parseCommaSeparated(getEnvOrDefault("CORS_ALLOWED_HEADERS", "Content-Type,Authorization")),
			AllowCredentials: getBoolOrDefault("CORS_ALLOW_CREDENTIALS", true),
			MaxAge:           getIntEnvOrDefault("CORS_MAX_AGE", 3600),
		},
		Storage: storage.Config{
			Type:           getEnvOrDefault("STORAGE_TYPE", "local"),
			LocalBaseDir:   getEnvOrDefault("STORAGE_LOCAL_BASE_DIR", "./bucket"),
			LocalPublicURL: getEnvOrDefault("STORAGE_LOCAL_PUBLIC_URL", getEnvOrDefault("SERVICE_URL", fmt.Sprintf("http://localhost:%d", serverPort))),
			S3Endpoint:     getEnvOrDefault("STORAGE_S3_ENDPOINT", ""),
			S3Bucket:       getEnvOrDefault("STORAGE_S3_BUCKET", "nsw-uploads"),
			S3Region:       getEnvOrDefault("STORAGE_S3_REGION", "us-east-1"),
			S3AccessKey:    getEnvOrDefault("STORAGE_S3_ACCESS_KEY", ""),
			S3SecretKey:    getEnvOrDefault("STORAGE_S3_SECRET_KEY", ""),
			S3UseSSL:       getBoolOrDefault("STORAGE_S3_USE_SSL", true),
			S3PublicURL:    getEnvOrDefault("STORAGE_S3_PUBLIC_URL", ""),
			LocalPutSecret: getEnvOrDefault("STORAGE_LOCAL_PUT_SECRET", "local-dev-secret"),
			PresignTTL:     getDurationOrDefault("STORAGE_PRESIGN_TTL", 15*time.Minute),
		},
		Authn: authn.Config{
			JWKSURL:               getEnvOrDefault("AUTH_JWKS_URL", "https://localhost:8090/oauth2/jwks"),
			Issuer:                getEnvOrDefault("AUTH_ISSUER", "https://localhost:8090"),
			Audience:              getEnvOrDefault("AUTH_AUDIENCE", "NSW_API"),
			ClientIDs:             parseCommaSeparated(getEnvOrDefault("AUTH_CLIENT_IDS", "TRADER_PORTAL_APP,FCAU_TO_NSW,NPQS_TO_NSW,CDA_TO_NSW,SLPA_TO_NSW")),
			InsecureSkipTLSVerify: getBoolOrDefault("AUTH_JWKS_INSECURE_SKIP_VERIFY", false),
		},
		Notification: notification.Config{
			Path: getEnvOrDefault("NOTIFICATIONS_CONFIG_PATH", "configs/notification.json"),
		},
		Temporal: temporal.Config{
			Host:      getEnvOrDefault("TEMPORAL_HOST", "localhost"),
			Port:      getIntEnvOrDefault("TEMPORAL_PORT", 7233),
			Namespace: getEnvOrDefault("TEMPORAL_NAMESPACE", "default"),
		},
		Audit: audit.Config{
			BaseURL: getEnvOrDefault("ARGUS_SERVICE_URL", ""),
			APIKey:  os.Getenv("ARGUS_API_KEY"),
		},
		ArtifactLoader: loaders.Config{
			Type: getEnvOrDefault("ARTIFACT_LOADER_TYPE", loaders.TypeLocal),
			Local: local.Config{
				Root: getEnvOrDefault("ARTIFACT_LOCAL_ROOT", "configs"),
			},
			GitHub: github.Config{
				Owner:      getEnvOrDefault("ARTIFACT_GITHUB_OWNER", ""),
				Repo:       getEnvOrDefault("ARTIFACT_GITHUB_REPO", ""),
				Ref:        getEnvOrDefault("ARTIFACT_GITHUB_REF", ""),
				BasePath:   getEnvOrDefault("ARTIFACT_GITHUB_BASE_PATH", ""),
				Token:      os.Getenv("ARTIFACT_GITHUB_TOKEN"),
				BaseURL:    getEnvOrDefault("ARTIFACT_GITHUB_BASE_URL", ""),
				UseRawHost: getBoolOrDefault("ARTIFACT_GITHUB_USE_RAW_HOST", false),
				RawBaseURL: getEnvOrDefault("ARTIFACT_GITHUB_RAW_BASE_URL", ""),
			},
			S3: s3.Config{
				Bucket:    getEnvOrDefault("ARTIFACT_S3_BUCKET", ""),
				Region:    getEnvOrDefault("ARTIFACT_S3_REGION", ""),
				Endpoint:  getEnvOrDefault("ARTIFACT_S3_ENDPOINT", ""),
				AccessKey: getEnvOrDefault("ARTIFACT_S3_ACCESS_KEY", ""),
				SecretKey: getEnvOrDefault("ARTIFACT_S3_SECRET_KEY", ""),
				Prefix:    getEnvOrDefault("ARTIFACT_S3_PREFIX", ""),
			},
		},
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that all required configuration is present.
func (c *Config) Validate() error {
	if c.Server.ServiceURL == "" {
		return fmt.Errorf("SERVICE_URL is required")
	}
	if err := HTTPURL("SERVICE_URL", c.Server.ServiceURL); err != nil {
		return err
	}
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("invalid storage configuration: %w", err)
	}
	if err := c.Authn.Validate(); err != nil {
		return fmt.Errorf("invalid authn configuration: %w", err)
	}
	if err := c.Temporal.Validate(); err != nil {
		return fmt.Errorf("invalid temporal configuration: %w", err)
	}
	if err := c.CORS.Validate(); err != nil {
		return fmt.Errorf("invalid CORS configuration: %w", err)
	}
	for _, origin := range c.CORS.AllowedOrigins {
		if origin == "*" && c.CORS.AllowCredentials {
			return fmt.Errorf("invalid CORS configuration: wildcard origin '*' is not allowed when AllowCredentials is true")
		}
	}
	if err := c.Notification.Validate(); err != nil {
		return fmt.Errorf("invalid notification configuration: %w", err)
	}
	if err := c.ArtifactLoader.Validate(); err != nil {
		return fmt.Errorf("invalid artifact loader configuration: %w", err)
	}
	return nil
}

// getEnvOrDefault returns the trimmed value of an environment variable or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

// getIntEnvOrDefault returns the integer value of an environment variable or a default value.
// Invalid values are silently ignored and the default is returned.
func getIntEnvOrDefault(key string, defaultValue int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getBoolOrDefault returns the boolean value of an environment variable or a default value.
// Invalid values are silently ignored and the default is returned.
func getBoolOrDefault(key string, defaultValue bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getDurationOrDefault returns the time.Duration value of an environment variable or a default value.
// Invalid values are silently ignored and the default is returned.
func getDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

// parseCommaSeparated splits a comma-separated string into a slice of trimmed strings.
func parseCommaSeparated(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
