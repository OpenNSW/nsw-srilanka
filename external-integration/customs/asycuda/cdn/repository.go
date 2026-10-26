package cdn

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DispatchNoteRepository defines the persistence interface for DispatchNote entities.
type DispatchNoteRepository interface {
	GetByEdgeID(ctx context.Context, edgeID string) (*DispatchNote, error)
	GetByCDNRef(ctx context.Context, ref DocumentReference) (*DispatchNote, error)
	Update(ctx context.Context, note *DispatchNote) error
	// RecordSubmitted records that SLC Edge accepted a submission as edgeID,
	// from workflow workflowID of case caseID, so its callbacks can find the
	// task that workflow parks. It is the only writer that creates the row: SLC
	// Edge issues a new edgeID for every submission and calls back only after
	// returning it, so a second call for the same edgeID comes from a retry of
	// the step, carries the same link, and leaves the row as it is.
	RecordSubmitted(ctx context.Context, edgeID, caseID, workflowID string) error
}

type dispatchNoteRepository struct {
	db *gorm.DB
}

// NewDispatchNoteRepository creates a GORM-backed DispatchNoteRepository.
func NewDispatchNoteRepository(db *gorm.DB) DispatchNoteRepository {
	return &dispatchNoteRepository{db: db}
}

func (r *dispatchNoteRepository) GetByEdgeID(ctx context.Context, edgeID string) (*DispatchNote, error) {
	var note DispatchNote
	if err := r.db.WithContext(ctx).Where("edge_id = ?", edgeID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &note, nil
}

func (r *dispatchNoteRepository) GetByCDNRef(ctx context.Context, ref DocumentReference) (*DispatchNote, error) {
	var note DispatchNote
	err := r.db.WithContext(ctx).
		Where("cdn_year = ? AND cdn_office = ? AND cdn_serial = ? AND cdn_number = ?",
			ref.Year, ref.Office, ref.Serial, ref.Number).
		First(&note).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &note, nil
}

func (r *dispatchNoteRepository) RecordSubmitted(ctx context.Context, edgeID, caseID, workflowID string) error {
	note := DispatchNote{
		ID:         uuid.NewString(),
		EdgeID:     edgeID,
		Status:     DispatchNoteStatusSubmitted,
		CaseID:     caseID,
		WorkflowID: workflowID,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "edge_id"}},
		DoNothing: true,
	}).Create(&note).Error
}

func (r *dispatchNoteRepository) Update(ctx context.Context, note *DispatchNote) error {
	return r.db.WithContext(ctx).Model(note).
		Select("status", "cdn_year", "cdn_office", "cdn_serial", "cdn_number", "errors", "updated_at").
		Updates(note).Error
}
