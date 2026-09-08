package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/OpenNSW/core/database"
)

// Config holds the configuration for the otc CLI.
type Config struct {
	Database database.Config
}

// Load reads the minimal configuration needed by the otc CLI from
// environment variables: database access only. It intentionally does not
// share cmd/server/config, which also requires and validates unrelated
// server settings (auth, CORS, temporal, artifact loading, etc.) that this
// CLI never uses.
func Load() (*Config, error) {
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
	}

	if err := cfg.Database.Validate(); err != nil {
		return nil, fmt.Errorf("invalid database configuration: %w", err)
	}

	return cfg, nil
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
