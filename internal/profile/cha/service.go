package cha

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Service defines operations for CHA profile management.
type Service interface {
	// GetByID retrieves a CHA record by its ID.
	// Returns ErrCHANotFound if no record exists.
	GetByID(ctx context.Context, id string) (*Record, error)

	// GetByEmail retrieves a CHA record by its email address.
	// Returns ErrCHANotFound if no record exists.
	GetByEmail(ctx context.Context, email string) (*Record, error)

	// List returns all CHA records ordered by name.
	List(ctx context.Context) ([]Record, error)

	// Health checks if the service can access the database.
	Health(ctx context.Context) error
}

type service struct {
	db *gorm.DB
}

// NewService creates a new CHA service instance.
func NewService(db *gorm.DB) Service {
	return &service{db: db}
}

func (s *service) GetByID(ctx context.Context, id string) (*Record, error) {
	if id == "" {
		return nil, ErrInvalidCHAID
	}

	var record Record
	result := s.db.WithContext(ctx).First(&record, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			slog.Debug("CHA record not found", "id", id)
			return nil, ErrCHANotFound
		}
		slog.Error("failed to fetch CHA record", "id", id, "error", result.Error)
		return nil, fmt.Errorf("database query failed: %w", result.Error)
	}

	return &record, nil
}

func (s *service) GetByEmail(ctx context.Context, email string) (*Record, error) {
	if email == "" {
		return nil, ErrInvalidEmail
	}
	var record Record
	result := s.db.WithContext(ctx).Where("email = ?", email).First(&record)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			slog.Debug("CHA record not found", "email", email)
			return nil, ErrCHANotFound
		}
		slog.Error("failed to fetch CHA record", "email", email, "error", result.Error)
		return nil, fmt.Errorf("database query failed: %w", result.Error)
	}

	return &record, nil
}

func (s *service) List(ctx context.Context) ([]Record, error) {
	var records []Record
	if err := s.db.WithContext(ctx).Order("name ASC").Find(&records).Error; err != nil {
		slog.Error("failed to list CHA records", "error", err)
		return nil, fmt.Errorf("failed to retrieve CHA records: %w", err)
	}
	return records, nil
}

func (s *service) Health(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		slog.Error("failed to retrieve underlying sql db", "error", err)
		return fmt.Errorf("failed to retrieve database: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		slog.Error("CHA service health check failed", "error", err)
		return fmt.Errorf("CHA service health check failed: %w", err)
	}
	return nil
}
