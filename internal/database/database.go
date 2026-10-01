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

// Open connects to the database described by cfg and wraps the connection
// pool in a GORM handle. Only the Postgres driver is supported.
func Open(ctx context.Context, cfg database.Config) (*gorm.DB, error) {
	if cfg.Driver != database.Postgres {
		return nil, fmt.Errorf("unsupported database driver %q: only %q is supported", cfg.Driver, database.Postgres)
	}

	sqlDB, err := database.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
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

	return database.HealthCheck(ctx, sqlDB)
}
