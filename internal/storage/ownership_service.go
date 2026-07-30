package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StorageObjectOwnership maps a storage key to workflow ownership identifiers.
type StorageObjectOwnership struct {
	Key           string `gorm:"primaryKey;column:key"`
	TaskID        string `gorm:"column:task_id"`
	ConsignmentID string `gorm:"column:consignment_id"`
	UploadedBy    string `gorm:"column:uploaded_by"`
}

func (StorageObjectOwnership) TableName() string {
	return "storage_object_ownership"
}

// ConsignmentOwnershipGetter retrieves trader and CHA company ownership for a consignment.
type ConsignmentOwnershipGetter interface {
	GetOwnership(ctx context.Context, consignmentID string) (traderCompanyID, chaCompanyID string, err error)
}

// OwnershipService coordinates persistent storage key ownership and access evaluation.
type OwnershipService struct {
	db                 *gorm.DB
	consignmentService ConsignmentOwnershipGetter
}

func NewOwnershipService(db *gorm.DB, consignmentService ConsignmentOwnershipGetter) *OwnershipService {
	return &OwnershipService{
		db:                 db,
		consignmentService: consignmentService,
	}
}

// RecordOwnership stores key ownership metadata upon file upload.
func (s *OwnershipService) RecordOwnership(ctx context.Context, key, taskID, consignmentID, uploadedBy string) error {
	rec := StorageObjectOwnership{
		Key:           key,
		TaskID:        taskID,
		ConsignmentID: consignmentID,
		UploadedBy:    uploadedBy,
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rec).Error
}

// OnUpload is a callback invoked when a file upload metadata object is created via the storage HTTP handler.
func (s *OwnershipService) OnUpload(ctx context.Context, metadata *storage.FileMetadata, authCtx *authn.AuthContext) error {
	if metadata == nil {
		return nil
	}
	var taskID, consignmentID, uploadedBy string
	if authCtx != nil && authCtx.User != nil {
		uploadedBy = authCtx.User.ID
	}
	if metadata.Ownership != nil {
		if tid, ok := metadata.Ownership["task_id"].(string); ok {
			taskID = tid
		}
		if cid, ok := metadata.Ownership["consignment_id"].(string); ok {
			consignmentID = cid
		}
	}
	return s.RecordOwnership(ctx, metadata.Key, taskID, consignmentID, uploadedBy)
}

// ValidateAccess verifies whether the authenticated caller is authorized to access the file key.
func (s *OwnershipService) ValidateAccess(ctx context.Context, key string, authCtx *authn.AuthContext) (bool, error) {
	if authCtx == nil || authCtx.User == nil {
		return false, nil
	}

	var rec StorageObjectOwnership
	err := s.db.WithContext(ctx).First(&rec, "key = ?", key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Key has no recorded ownership constraint; deny access (fail-closed)
			return false, nil
		}
		return false, fmt.Errorf("failed to query storage ownership: %w", err)
	}

	// If uploaded_by matches caller ID directly, allow
	if rec.UploadedBy != "" && rec.UploadedBy == authCtx.User.ID {
		return true, nil
	}

	// Agency Officers can access any file attached to an agency workflow
	if isAgencyUser(authCtx) {
		return true, nil
	}

	return s.validateConsignmentAccess(ctx, &rec, authCtx.User.OUHandle)
}

func (s *OwnershipService) validateConsignmentAccess(ctx context.Context, rec *StorageObjectOwnership, userCompany string) (bool, error) {
	cid := rec.ConsignmentID
	if cid == "" && rec.TaskID != "" {
		cid = s.resolveConsignmentIDFromTask(ctx, rec.TaskID)
	}

	if cid == "" || s.consignmentService == nil {
		return true, nil
	}

	traderID, chaID, err := s.consignmentService.GetOwnership(ctx, cid)
	if err != nil {
		return false, fmt.Errorf("failed to check consignment ownership for %s: %w", cid, err)
	}

	if userCompany != "" && (userCompany == traderID || userCompany == chaID) {
		return true, nil
	}

	return false, nil
}

func (s *OwnershipService) resolveConsignmentIDFromTask(ctx context.Context, taskID string) string {
	var parentWorkflowID string
	err := s.db.WithContext(ctx).
		Table("task_records").
		Select("parent_workflow_id").
		Where("id = ?", taskID).
		Scan(&parentWorkflowID).Error
	if err != nil || parentWorkflowID == "" {
		return taskID
	}
	return parentWorkflowID
}

func isAgencyUser(authCtx *authn.AuthContext) bool {
	if authCtx == nil || authCtx.User == nil {
		return false
	}
	for _, role := range authCtx.User.Roles {
		switch role {
		case "Officer", "AGENCY_OFFICER", "Admin", "reviewer", "lab_officer", "lab_manager":
			return true
		}
	}
	return false
}
