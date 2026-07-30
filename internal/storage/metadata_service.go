package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ObjectMetadata stores storage key metadata and its associated workflow identifiers.
type ObjectMetadata struct {
	Key           string    `gorm:"primaryKey;column:key"`
	TaskID        string    `gorm:"column:task_id"`
	ConsignmentID string    `gorm:"column:consignment_id"`
	CompanyID     string    `gorm:"column:company_id"`
	UploadedBy    string    `gorm:"column:uploaded_by"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (ObjectMetadata) TableName() string {
	return "storage_object_metadata"
}

// TaskAuthorizer evaluates whether an authenticated caller is authorized to access a specific task.
type TaskAuthorizer interface {
	CanAccessTask(ctx context.Context, taskID string, authCtx *authn.AuthContext) (bool, error)
}

// ObjectMetadataService manages persistent storage key metadata and access evaluation.
type ObjectMetadataService struct {
	db             *gorm.DB
	taskAuthorizer TaskAuthorizer
}

func NewObjectMetadataService(db *gorm.DB, taskAuthorizer TaskAuthorizer) *ObjectMetadataService {
	return &ObjectMetadataService{
		db:             db,
		taskAuthorizer: taskAuthorizer,
	}
}

// RecordMetadata stores object key metadata upon file upload or workflow association.
func (s *ObjectMetadataService) RecordMetadata(ctx context.Context, key, taskID, consignmentID, companyID, uploadedBy string) error {
	rec := ObjectMetadata{
		Key:           key,
		TaskID:        taskID,
		ConsignmentID: consignmentID,
		CompanyID:     companyID,
		UploadedBy:    uploadedBy,
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "key"}},
			DoUpdates: clause.Assignments(map[string]any{
				"task_id":        gorm.Expr("COALESCE(NULLIF(EXCLUDED.task_id, ''), storage_object_metadata.task_id)"),
				"consignment_id": gorm.Expr("COALESCE(NULLIF(EXCLUDED.consignment_id, ''), storage_object_metadata.consignment_id)"),
				"company_id":     gorm.Expr("COALESCE(NULLIF(EXCLUDED.company_id, ''), storage_object_metadata.company_id)"),
				"uploaded_by":    gorm.Expr("COALESCE(NULLIF(EXCLUDED.uploaded_by, ''), storage_object_metadata.uploaded_by)"),
			}),
		}).
		Create(&rec).Error
}

// OnUpload is a callback invoked when a presigned upload URL is generated.
func (s *ObjectMetadataService) OnUpload(ctx context.Context, metadata *storage.FileMetadata, authCtx *authn.AuthContext) error {
	if metadata == nil {
		return nil
	}
	var uploadedBy, companyID string
	if authCtx != nil && authCtx.User != nil {
		uploadedBy = authCtx.User.ID
		companyID = authCtx.User.OUHandle
	}
	return s.RecordMetadata(ctx, metadata.Key, "", "", companyID, uploadedBy)
}

// ValidateAccess verifies whether the authenticated caller is authorized to access the file key.
func (s *ObjectMetadataService) ValidateAccess(ctx context.Context, key string, authCtx *authn.AuthContext) (bool, error) {
	if authCtx == nil || authCtx.User == nil {
		return false, nil
	}

	var rec ObjectMetadata
	err := s.db.WithContext(ctx).First(&rec, "key = ?", key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Key has no recorded metadata; deny access (fail-closed)
			return false, nil
		}
		return false, fmt.Errorf("failed to query storage object metadata: %w", err)
	}

	// 1. Uploader is authorized directly
	if rec.UploadedBy != "" && authCtx.User.ID != "" && rec.UploadedBy == authCtx.User.ID {
		return true, nil
	}

	// 2. Company-level authorization for Trader/CHA company files
	if rec.CompanyID != "" && authCtx.User.OUHandle != "" && rec.CompanyID == authCtx.User.OUHandle {
		return true, nil
	}

	// 3. Delegate task-level authorization (checks taskAuthz configuration for responsible agency/roles)
	if rec.TaskID != "" && s.taskAuthorizer != nil {
		return s.taskAuthorizer.CanAccessTask(ctx, rec.TaskID, authCtx)
	}

	return false, nil
}
