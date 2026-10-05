package database

import (
	"fmt"

	"github.com/OpenNSW/core/database"
)

// Validate checks the db section of config.yaml: core/database's own checks,
// plus the postgres settings it leaves optional but this deployment must set,
// since there are no built-in defaults to fall back on. An unset sslMode would
// let the driver fall back to "prefer", i.e. silently connect in plaintext
// when TLS fails.
func Validate(cfg database.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.Driver != database.Postgres {
		return nil // Open rejects it with the supported driver named
	}
	if p := cfg.Postgres.Port; p < 1 || p > 65535 {
		return fmt.Errorf("db.postgres.port must be between 1 and 65535, got %d", p)
	}
	if cfg.Postgres.SSLMode == "" {
		return fmt.Errorf("db.postgres.sslMode is required")
	}
	return nil
}
