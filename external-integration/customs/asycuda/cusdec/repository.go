package cusdec

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// DeclarationRepository defines the persistence interface for CusdecDeclaration entities.
type DeclarationRepository interface {
	GetByEdgeID(ctx context.Context, edgeID string) (*CusdecDeclaration, error)
	Create(ctx context.Context, decl *CusdecDeclaration) error
	Update(ctx context.Context, decl *CusdecDeclaration) error
}

type declarationRepository struct {
	db *gorm.DB
}

// NewDeclarationRepository creates a GORM-backed DeclarationRepository.
func NewDeclarationRepository(db *gorm.DB) DeclarationRepository {
	return &declarationRepository{db: db}
}

func (r *declarationRepository) GetByEdgeID(ctx context.Context, edgeID string) (*CusdecDeclaration, error) {
	var decl CusdecDeclaration
	if err := r.db.WithContext(ctx).Where("edge_id = ?", edgeID).First(&decl).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &decl, nil
}

func (r *declarationRepository) Create(ctx context.Context, decl *CusdecDeclaration) error {
	return r.db.WithContext(ctx).Create(decl).Error
}

func (r *declarationRepository) Update(ctx context.Context, decl *CusdecDeclaration) error {
	return r.db.WithContext(ctx).Model(decl).
		Select("status", "cusdec_year", "cusdec_office", "cusdec_serial", "cusdec_number", "errors", "updated_at").
		Updates(decl).Error
}
