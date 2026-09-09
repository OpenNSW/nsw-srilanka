package company

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/OpenNSW/core/pagination"
)

// pgUniqueViolationCode is the PostgreSQL error code for a unique-constraint violation (23505).
const pgUniqueViolationCode = "23505"

// Service defines operations for company profile management.
type Service interface {
	// GetCompanyByID retrieves a company record by its ID.
	// Returns ErrCompanyNotFound if no record exists.
	GetCompanyByID(ctx context.Context, id string) (*Record, error)

	// GetCompanyByOUHandle retrieves a company record by its IdP organisational unit handle.
	// Returns ErrCompanyNotFound if no record exists.
	GetCompanyByOUHandle(ctx context.Context, ouHandle string) (*Record, error)

	// ListCompanies returns a paginated list of companies ordered by name, optionally filtered by
	// HasCHA and a case-insensitive substring match on Name.
	ListCompanies(ctx context.Context, filter ListFilter) (*ListResult, error)

	// UpdateCompany performs an append-only merge of data into the company's Data field.
	// New keys are added and existing keys are replaced only when explicitly provided.
	// Keys absent from data are never removed.
	// Returns ErrCompanyNotFound if the company does not exist.
	UpdateCompany(ctx context.Context, id string, data map[string]any) error

	// UpdateCompanyFields updates the mutable core fields (Name, OUHandle, HasCHA) of a company
	// record. Only fields set (non-nil) in fields are changed; the rest are left untouched.
	// Returns ErrCompanyNotFound if the company does not exist, or ErrOUHandleConflict if the
	// requested OUHandle is already used by another company.
	UpdateCompanyFields(ctx context.Context, id string, fields CompanyFieldsUpdate) error

	// ReplaceCompanyData replaces the company's entire Data field with the given JSON document.
	// Unlike UpdateCompany, this is not a merge: keys absent from data are removed. data must be
	// a valid JSON object. Returns ErrCompanyNotFound if the company does not exist.
	ReplaceCompanyData(ctx context.Context, id string, data json.RawMessage) error

	// UpsertCompany creates the company record if its ID does not exist, or replaces Name,
	// OUHandle, HasCHA, Data, and UpdatedAt if it does, as a single atomic INSERT ... ON CONFLICT
	// statement (safe under concurrent callers, e.g. declarative bulk loading via
	// `otc company apply`). Returns ErrOUHandleConflict if OUHandle collides with a
	// different company's existing OUHandle.
	UpsertCompany(ctx context.Context, record *Record) error

	// Health checks if the service can access the database.
	Health(ctx context.Context) error

	// CreateCompany creates a new company record in the database.
	CreateCompany(ctx context.Context, record *Record) error
}

type service struct {
	db *gorm.DB
}

// NewService creates a new company service instance.
// TranslateError is enabled on a cloned session so unique-constraint violations
// surface as gorm.ErrDuplicatedKey without new call sites importing a SQL driver.
func NewService(db *gorm.DB) Service {
	if db != nil {
		db = db.Session(&gorm.Session{})
		db.TranslateError = true
	}
	return &service{db: db}
}

func (s *service) getByID(ctx context.Context, id string) (*Record, error) {
	var record Record
	result := s.db.WithContext(ctx).Where("id = ?", id).First(&record)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			slog.Debug("company record not found", "id", id)
			return nil, ErrCompanyNotFound
		}
		slog.Error("failed to fetch company record", "id", id, "error", result.Error)
		return nil, fmt.Errorf("database query failed: %w", result.Error)
	}
	return &record, nil
}

func (s *service) getByOUHandle(ctx context.Context, handle string) (*Record, error) {
	var record Record
	result := s.db.WithContext(ctx).Where("ou_handle = ?", handle).First(&record)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			slog.Debug("company record not found", "ou_handle", handle)
			return nil, ErrCompanyNotFound
		}
		slog.Error("failed to fetch company record", "ou_handle", handle, "error", result.Error)
		return nil, fmt.Errorf("database query failed: %w", result.Error)
	}
	return &record, nil
}

func (s *service) GetCompanyByID(ctx context.Context, id string) (*Record, error) {
	if id == "" {
		return nil, ErrInvalidCompanyID
	}
	return s.getByID(ctx, id)
}

func (s *service) GetCompanyByOUHandle(ctx context.Context, ouHandle string) (*Record, error) {
	if ouHandle == "" {
		return nil, ErrInvalidCompanyID
	}
	return s.getByOUHandle(ctx, ouHandle)
}

func (s *service) ListCompanies(ctx context.Context, filter ListFilter) (*ListResult, error) {
	finalOffset, finalLimit := pagination.ResolvePaginationParams(filter.Offset, filter.Limit)

	// baseQuery builds a fresh *gorm.DB with only the filter conditions applied.
	// A new instance is constructed each time to prevent GORM's shared Clauses map
	// from being contaminated by pagination (LIMIT/OFFSET) set on the list query.
	baseQuery := func() *gorm.DB {
		q := s.db.WithContext(ctx).Model(&Record{})
		if filter.HasCHA != nil {
			q = q.Where("has_cha = ?", *filter.HasCHA)
		}
		if filter.Name != nil {
			name := strings.TrimSpace(*filter.Name)
			if name != "" {
				q = q.Where("name ILIKE ?", "%"+name+"%")
			}
		}
		return q
	}

	var items []Summary
	if err := baseQuery().Select("id, name, has_cha").Order("name ASC").Offset(finalOffset).Limit(finalLimit).Scan(&items).Error; err != nil {
		slog.Error("failed to list company records", "error", err)
		return nil, fmt.Errorf("database query failed: %w", err)
	}

	var totalCount int64
	if len(items) < finalLimit && finalOffset == 0 {
		totalCount = int64(len(items))
	} else {
		if err := baseQuery().Count(&totalCount).Error; err != nil {
			slog.Error("failed to count company records", "error", err)
			return nil, fmt.Errorf("database query failed: %w", err)
		}
	}

	result := pagination.NewPageResult(items, totalCount, finalOffset, finalLimit)
	return &result, nil
}

func (s *service) UpdateCompany(ctx context.Context, id string, data map[string]any) error {
	if id == "" {
		return ErrInvalidCompanyID
	}

	if len(data) == 0 {
		return nil
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal company data: %w", err)
	}

	// Use PostgreSQL's JSONB concatenation operator (||) for an atomic merge.
	// This avoids race conditions inherent in a read-modify-write cycle.
	result := s.db.WithContext(ctx).Model(&Record{}).
		Where("id = ?", id).
		Update("data", gorm.Expr("data || ?::jsonb", string(jsonBytes)))

	if result.Error != nil {
		slog.Error("failed to update company data", "id", id, "error", result.Error)
		return fmt.Errorf("failed to update company data: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrCompanyNotFound
	}

	return nil
}

func (s *service) UpdateCompanyFields(ctx context.Context, id string, fields CompanyFieldsUpdate) error {
	if id == "" {
		return ErrInvalidCompanyID
	}

	if fields.Name != nil && strings.TrimSpace(*fields.Name) == "" {
		return fmt.Errorf("company name cannot be empty")
	}
	if fields.OUHandle != nil && strings.TrimSpace(*fields.OUHandle) == "" {
		return fmt.Errorf("ou_handle cannot be empty")
	}

	updates := make(map[string]any, 3)
	if fields.Name != nil {
		updates["name"] = *fields.Name
	}
	if fields.OUHandle != nil {
		updates["ou_handle"] = *fields.OUHandle
	}
	if fields.HasCHA != nil {
		updates["has_cha"] = *fields.HasCHA
	}
	if len(updates) == 0 {
		return nil
	}

	result := s.db.WithContext(ctx).Model(&Record{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(result.Error, &pgErr) && pgErr.Code == pgUniqueViolationCode {
			return ErrOUHandleConflict
		}
		slog.Error("failed to update company fields", "id", id, "error", result.Error)
		return fmt.Errorf("failed to update company fields: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrCompanyNotFound
	}

	return nil
}

func (s *service) ReplaceCompanyData(ctx context.Context, id string, data json.RawMessage) error {
	if id == "" {
		return ErrInvalidCompanyID
	}

	if len(data) == 0 {
		data = json.RawMessage("{}")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return fmt.Errorf("company data must be a valid JSON object")
	}

	result := s.db.WithContext(ctx).Model(&Record{}).
		Where("id = ?", id).
		Update("data", gorm.Expr("?::jsonb", string(data)))

	if result.Error != nil {
		slog.Error("failed to replace company data", "id", id, "error", result.Error)
		return fmt.Errorf("failed to replace company data: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrCompanyNotFound
	}

	return nil
}

func (s *service) Health(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		slog.Error("failed to retrieve underlying sql db", "error", err)
		return fmt.Errorf("failed to retrieve database: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		slog.Error("company service health check failed", "error", err)
		return fmt.Errorf("company service health check failed: %w", err)
	}
	return nil
}

func (s *service) UpsertCompany(ctx context.Context, record *Record) error {
	if record.ID == "" {
		return ErrInvalidCompanyID
	}
	if record.Name == "" {
		return fmt.Errorf("company name is required")
	}
	if record.OUHandle == "" {
		record.OUHandle = record.ID
	}
	if len(record.Data) == 0 {
		record.Data = json.RawMessage("{}")
	}

	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "ou_handle", "has_cha", "data", "updated_at"}),
	}).Create(record)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return ErrOUHandleConflict
		}
		slog.Error("failed to upsert company record", "id", record.ID, "error", result.Error)
		return fmt.Errorf("failed to upsert company record: %w", result.Error)
	}

	return nil
}

func (s *service) CreateCompany(ctx context.Context, record *Record) error {
	if record.ID == "" {
		return ErrInvalidCompanyID
	}
	if record.Name == "" {
		return fmt.Errorf("company name is required")
	}
	if record.OUHandle == "" {
		record.OUHandle = record.ID
	}

	result := s.db.WithContext(ctx).Create(record)
	if result.Error != nil {
		slog.Error("failed to create company record", "id", record.ID, "error", result.Error)
		return fmt.Errorf("database insert failed: %w", result.Error)
	}
	return nil
}
