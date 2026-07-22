package asycuda

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// DispatchNoteRepository defines the persistence interface for DispatchNote
// entities. Implementations must handle context cancellation and propagate
// errors from the underlying store.
type DispatchNoteRepository interface {
	// GetByEdgID retrieves a dispatch note by its correlation UUID (edgId)
	// assigned during the initial submission to ASYCUDA. Returns (nil, nil)
	// when no matching record exists.
	GetByEdgID(ctx context.Context, edgID string) (*DispatchNote, error)

	// GetByCDNRef retrieves a dispatch note by its composite CDN reference
	// (year + office + serial + number). Returns (nil, nil) when no matching
	// record exists.
	GetByCDNRef(ctx context.Context, ref DocumentReference) (*DispatchNote, error)

	// Update persists all changed fields of the given dispatch note.
	Update(ctx context.Context, note *DispatchNote) error
}

type dispatchNoteRepository struct {
	db *gorm.DB
}

// NewDispatchNoteRepository creates a GORM-backed DispatchNoteRepository.
func NewDispatchNoteRepository(db *gorm.DB) DispatchNoteRepository {
	return &dispatchNoteRepository{db: db}
}

func (r *dispatchNoteRepository) GetByEdgID(ctx context.Context, edgID string) (*DispatchNote, error) {
	var note DispatchNote
	if err := r.db.WithContext(ctx).Where("edg_id = ?", edgID).First(&note).Error; err != nil {
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

func (r *dispatchNoteRepository) Update(ctx context.Context, note *DispatchNote) error {
	return r.db.WithContext(ctx).Model(note).
		Select("status", "cdn_year", "cdn_office", "cdn_serial", "cdn_number", "errors", "updated_at").
		Updates(note).Error
}
