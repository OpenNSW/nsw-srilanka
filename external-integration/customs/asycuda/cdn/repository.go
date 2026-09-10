package cdn

import (
	"context"
	"errors"
	"log/slog"

	"gorm.io/gorm"
)

// DispatchNoteRepository defines the persistence interface for DispatchNote entities.
type DispatchNoteRepository interface {
	GetByEdgeID(ctx context.Context, edgeID string) (*DispatchNote, error)
	GetByCDNRef(ctx context.Context, ref DocumentReference) (*DispatchNote, error)
	Create(ctx context.Context, note *DispatchNote) error
	Update(ctx context.Context, note *DispatchNote) error
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
			// Ordinary on a first callback; the caller distinguishes that from
			// a repeat, and both paths read very differently in the log.
			slog.DebugContext(ctx, "cdn: no dispatch note held for edgeId", "edge_id", edgeID)
			return nil, nil
		}
		slog.ErrorContext(ctx, "cdn: dispatch note lookup by edgeId failed", "edge_id", edgeID, "error", err)
		return nil, err
	}
	slog.DebugContext(ctx, "cdn: dispatch note found by edgeId",
		"edge_id", edgeID, "dispatch_note_id", note.ID, "status", note.Status)
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
			slog.DebugContext(ctx, "cdn: no dispatch note holds this reference", "cdn_ref", ref.String())
			return nil, nil
		}
		slog.ErrorContext(ctx, "cdn: dispatch note lookup by reference failed", "cdn_ref", ref.String(), "error", err)
		return nil, err
	}
	slog.DebugContext(ctx, "cdn: dispatch note found by reference",
		"cdn_ref", ref.String(), "dispatch_note_id", note.ID, "edge_id", note.EdgeID, "status", note.Status)
	return &note, nil
}

func (r *dispatchNoteRepository) Create(ctx context.Context, note *DispatchNote) error {
	if err := r.db.WithContext(ctx).Create(note).Error; err != nil {
		slog.ErrorContext(ctx, "cdn: dispatch note insert failed",
			"dispatch_note_id", note.ID, "edge_id", note.EdgeID, "error", err)
		return err
	}
	slog.InfoContext(ctx, "cdn: dispatch note inserted",
		"dispatch_note_id", note.ID, "edge_id", note.EdgeID, "status", note.Status)
	return nil
}

func (r *dispatchNoteRepository) Update(ctx context.Context, note *DispatchNote) error {
	err := r.db.WithContext(ctx).Model(note).
		Select("status", "cdn_year", "cdn_office", "cdn_serial", "cdn_number", "errors", "updated_at").
		Updates(note).Error
	if err != nil {
		slog.ErrorContext(ctx, "cdn: dispatch note update failed",
			"dispatch_note_id", note.ID, "edge_id", note.EdgeID, "error", err)
		return err
	}
	slog.InfoContext(ctx, "cdn: dispatch note updated",
		"dispatch_note_id", note.ID, "edge_id", note.EdgeID, "status", note.Status)
	return nil
}
