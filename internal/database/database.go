// Package database opens the application's GORM handle on top of the
// driver-agnostic core/database connection factory.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenNSW/core/database"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver core/database opens Postgres with
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Bounds on the connectivity checks, so an unresponsive server cannot block
// startup or a health probe. A shorter deadline already on the caller's
// context still applies.
const (
	openTimeout        = 10 * time.Second
	healthCheckTimeout = 5 * time.Second
)

// Open connects to the database described by cfg and wraps the connection
// pool in a GORM handle. Only the Postgres driver is supported.
func Open(ctx context.Context, cfg database.Config) (*gorm.DB, error) {
	if cfg.Driver != database.Postgres {
		return nil, fmt.Errorf("unsupported database driver %q: only %q is supported", cfg.Driver, database.Postgres)
	}

	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()

	sqlDB, err := database.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		// database.New has already pinged with ctx; GORM's own ping ignores it.
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Error),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		TranslateError: true,
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize gorm: %w", err)
	}

	return db, nil
}

// Close closes the connection pool underlying db.
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying database: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	slog.Info("database connection closed")
	return nil
}

// HealthCheck pings the database underlying db.
func HealthCheck(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying database: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	return database.HealthCheck(ctx, sqlDB)
}
