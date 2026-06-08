package user

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Service defines operations for user profile management.
// It handles database interactions for persisted user records.
type Service interface {
	// GetUser retrieves a user record by the user ID.
	// Returns a user record if found, nil if not found, or an error on failure.
	GetUser(id string) (*Record, error)

	// GetOrCreateUser creates or retrieves a user record by idpUserID.
	// Returns the user ID of the created or existing record, or an error on failure.
	// If err is non-nil, the returned user ID will be nil.
	GetOrCreateUser(idpUserID, email, phone, ouID, ouHandle string) (*string, error)

	// UpdateUserData updates the Data field for an existing user record.
	// The provided data should be valid JSON bytes.
	// Returns ErrUserNotFound if the user does not exist.
	UpdateUserData(id string, data []byte) error

	// Health checks if the service can access the database.
	Health() error
}

// service implements the Service interface using GORM.
type service struct {
	db *gorm.DB
}

// NewService creates a new user service instance.
func NewService(db *gorm.DB) Service {
	return &service{db: db}
}

// GetUser retrieves a user record from the database.
func (s *service) GetUser(id string) (*Record, error) {
	if id == "" {
		return nil, ErrInvalidUserID
	}

	var record Record
	result := s.db.Where("id = ?", id).First(&record)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			slog.Debug("user record not found", "id", id)
			return nil, ErrUserNotFound
		}
		slog.Error("failed to fetch user record", "id", id, "error", result.Error)
		return nil, fmt.Errorf("database query failed: %w", result.Error)
	}

	return &record, nil
}

// GetOrCreateUser creates a new user record in the database if one does not exist.
// Returns the existing user ID when a record already exists.
// The Data field is initialized to an empty JSON object for new records.
func (s *service) GetOrCreateUser(idpUserID, email, phone, ouID, ouHandle string) (*string, error) {
	if idpUserID == "" {
		return nil, ErrInvalidUserID
	}

	existingID, err := s.getUserIDByIDP(idpUserID)
	if err != nil {
		return nil, err
	}
	if existingID != nil {
		return existingID, nil
	}

	userID := uuid.New().String()
	record := &Record{
		ID:          userID,
		IDPUserID:   idpUserID,
		Email:       email,
		PhoneNumber: phone,
		OUID:        ouID,
		OUHandle:    ouHandle,
		Data:        []byte(`{}`),
	}

	result := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idp_user_id"}},
		DoNothing: true,
	}).Create(record)
	if result.Error != nil {
		slog.Error("failed to create user record", "id", record.ID, "error", result.Error)
		return nil, fmt.Errorf("database insert failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		duplicateID, lookupErr := s.getUserIDByIDP(idpUserID)
		switch {
		case lookupErr != nil:
			return nil, lookupErr
		case duplicateID != nil:
			slog.Debug("user record already exists after insert", "idp_user_id", idpUserID, "user_id", *duplicateID)
			return duplicateID, nil
		}
		return nil, fmt.Errorf("user record insert skipped but existing record not found")
	}

	slog.Debug("user record created", "id", record.ID, "email", email)
	return &record.ID, nil
}

// UpdateUserData updates the Data field for a user record.
func (s *service) UpdateUserData(userID string, data []byte) error {
	if userID == "" {
		return ErrInvalidUserID
	}

	result := s.db.Model(&Record{}).Where("id = ?", userID).Update("data", data)
	if result.Error != nil {
		slog.Error("failed to update user data", "id", userID, "error", result.Error)
		return fmt.Errorf("database update failed: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		slog.Debug("user record not found for update", "id", userID)
		return ErrUserNotFound
	}

	slog.Debug("user data updated", "id", userID)
	return nil
}

// getUserIDByIDP checks if a user record exists for the given idpUserID.
// Returns nil, nil if the user does not exist.
func (s *service) getUserIDByIDP(idpUserId string) (*string, error) {
	if idpUserId == "" {
		return nil, ErrInvalidUserID
	}

	var record Record
	result := s.db.Where("idp_user_id = ?", idpUserId).First(&record)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			slog.Debug("user record not found", "idp_user_id", idpUserId)
			return nil, nil
		}
		slog.Error("failed to fetch user record", "idp_user_id", idpUserId, "error", result.Error)
		return nil, fmt.Errorf("database query failed: %w", result.Error)
	}

	return &record.ID, nil
}

// Health checks if the service can access the database.
func (s *service) Health() error {
	var count int64
	result := s.db.Model(&Record{}).Count(&count)
	if result.Error != nil {
		slog.Error("user service health check failed", "error", result.Error)
		return fmt.Errorf("user service health check failed: %w", result.Error)
	}

	slog.Debug("user service health check passed", "accessible", true)
	return nil
}
