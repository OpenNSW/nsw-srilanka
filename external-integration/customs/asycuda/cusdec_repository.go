package asycuda

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// CusdecDeclarationRepository defines the persistence interface for CusdecDeclaration
// entities.
type CusdecDeclarationRepository interface {
	// GetByEdgeID retrieves a Customs Declaration by its correlation UUID (edgeId)
	// assigned during the initial submission. Returns (nil, nil) when no matching
	// record exists.
	GetByEdgeID(ctx context.Context, edgeID string) (*CusdecDeclaration, error)

	// Create persists a new Customs Declaration record.
	Create(ctx context.Context, decl *CusdecDeclaration) error

	// Update persists all changed fields of the given Customs Declaration record.
	Update(ctx context.Context, decl *CusdecDeclaration) error
}

type cusdecDeclarationRepository struct {
	db *gorm.DB
}

// NewCusdecDeclarationRepository creates a GORM-backed CusdecDeclarationRepository.
func NewCusdecDeclarationRepository(db *gorm.DB) CusdecDeclarationRepository {
	return &cusdecDeclarationRepository{db: db}
}

func (r *cusdecDeclarationRepository) GetByEdgeID(ctx context.Context, edgeID string) (*CusdecDeclaration, error) {
	var decl CusdecDeclaration
	if err := r.db.WithContext(ctx).Where("edge_id = ?", edgeID).First(&decl).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &decl, nil
}

func (r *cusdecDeclarationRepository) Create(ctx context.Context, decl *CusdecDeclaration) error {
	return r.db.WithContext(ctx).Create(decl).Error
}

func (r *cusdecDeclarationRepository) Update(ctx context.Context, decl *CusdecDeclaration) error {
	return r.db.WithContext(ctx).Model(decl).
		Select("status", "cusdec_year", "cusdec_office", "cusdec_serial", "cusdec_number", "errors").
		Updates(decl).Error
}
