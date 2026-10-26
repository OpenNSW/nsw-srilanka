package cusdec

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeclarationRepository defines the persistence interface for CusdecDeclaration entities.
type DeclarationRepository interface {
	GetByEdgeID(ctx context.Context, edgeID string) (*CusdecDeclaration, error)
	GetByCusdecRef(ctx context.Context, ref DocumentReference) (*CusdecDeclaration, error)
	Update(ctx context.Context, decl *CusdecDeclaration) error
	// RecordSubmitted records that SLC Edge accepted a submission as edgeID,
	// from workflow workflowID of case caseID, so its callbacks can find the
	// task that workflow parks. It is the only writer that creates the row: SLC
	// Edge issues a new edgeID for every submission and calls back only after
	// returning it, so a second call for the same edgeID comes from a retry of
	// the step, carries the same link, and leaves the row as it is.
	RecordSubmitted(ctx context.Context, edgeID, caseID, workflowID string) error
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

func (r *declarationRepository) GetByCusdecRef(ctx context.Context, ref DocumentReference) (*CusdecDeclaration, error) {
	var decl CusdecDeclaration
	err := r.db.WithContext(ctx).
		Where("cusdec_year = ? AND cusdec_office = ? AND cusdec_serial = ? AND cusdec_number = ?",
			ref.Year, ref.Office, ref.Serial, ref.Number).
		First(&decl).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &decl, nil
}

func (r *declarationRepository) RecordSubmitted(ctx context.Context, edgeID, caseID, workflowID string) error {
	decl := CusdecDeclaration{
		ID:         uuid.NewString(),
		EdgeID:     edgeID,
		Status:     CusdecStatusSubmitted,
		CaseID:     caseID,
		WorkflowID: workflowID,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "edge_id"}},
		DoNothing: true,
	}).Create(&decl).Error
}

func (r *declarationRepository) Update(ctx context.Context, decl *CusdecDeclaration) error {
	return r.db.WithContext(ctx).Model(decl).
		Select("status", "cusdec_year", "cusdec_office", "cusdec_serial", "cusdec_number", "errors", "updated_at").
		Updates(decl).Error
}
