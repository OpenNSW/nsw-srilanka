package database

import (
	"fmt"

	"github.com/OpenNSW/core/database"
)

// Validate checks the db section of config.yaml: core/database's own checks,
// plus the postgres settings it leaves optional but this deployment must set,
// since there are no built-in defaults to fall back on. An unset sslMode would
// let the driver fall back to "prefer", i.e. silently connect in plaintext
// when TLS fails; an unset pool setting would leave database/sql's default in
// place, which for maxOpenConns means no limit at all.
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
	pool := cfg.Postgres.Pool
	for _, f := range []struct {
		key   string
		value int
	}{
		{"db.postgres.pool.maxOpenConns", pool.MaxOpenConns},
		{"db.postgres.pool.maxIdleConns", pool.MaxIdleConns},
		{"db.postgres.pool.maxConnLifetimeSeconds", pool.MaxConnLifetimeSeconds},
	} {
		if f.value <= 0 {
			return fmt.Errorf("%s must be greater than zero, got %d", f.key, f.value)
		}
	}
	return nil
}
