package storage

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockTaskAuthorizer struct {
	allowed bool
	err     error
}

func (m *mockTaskAuthorizer) CanAccessTask(ctx context.Context, taskID string, authCtx *authn.AuthContext) (bool, error) {
	return m.allowed, m.err
}

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite test DB: %v", err)
	}

	migrationSQL, err := os.ReadFile("../../migrations/000011_create_storage_object_metadata.sql")
	if err != nil {
		t.Fatalf("failed to read migration SQL: %v", err)
	}

	if err := db.Exec(string(migrationSQL)).Error; err != nil {
		t.Fatalf("failed to execute migration SQL: %v", err)
	}
	return db
}

func TestObjectMetadataService_ValidateAccess(t *testing.T) {
	db := setupTestDB(t)
	authorizer := &mockTaskAuthorizer{allowed: true}
	svc := NewObjectMetadataService(db, authorizer)

	ctx := context.Background()
	key := "00000000-0000-0000-0000-000000000001.pdf"

	if err := svc.RecordMetadata(ctx, key, "task-1", "c-1", "company-trader-1", "user-uploader-1"); err != nil {
		t.Fatalf("RecordMetadata failed: %v", err)
	}

	t.Run("unauthenticated fails", func(t *testing.T) {
		allowed, err := svc.ValidateAccess(ctx, key, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("expected unauthenticated caller to be denied")
		}
	})

	t.Run("uploader allowed directly", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-uploader-1"}}
		allowed, err := svc.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Error("expected uploader to be allowed directly")
		}
	})

	t.Run("company allowed directly", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-colleague", OUHandle: "company-trader-1"}}
		allowed, err := svc.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Error("expected colleague from same company to be allowed")
		}
	})

	t.Run("task authorizer allowed", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-officer-2"}}
		allowed, err := svc.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Error("expected task authorizer to allow officer")
		}
	})

	t.Run("task authorizer denied", func(t *testing.T) {
		svcDenied := NewObjectMetadataService(db, &mockTaskAuthorizer{allowed: false})
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-unauthorized"}}
		allowed, err := svcDenied.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("expected unauthorized user to be denied access")
		}
	})

	t.Run("unmapped key denied", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-uploader-1"}}
		allowed, err := svc.ValidateAccess(ctx, "unmapped-key.pdf", authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("expected unmapped key to be denied access (fail-closed)")
		}
	})
}

func TestObjectMetadataService_OnUpload(t *testing.T) {
	db := setupTestDB(t)
	authorizer := &mockTaskAuthorizer{allowed: true}
	svc := NewObjectMetadataService(db, authorizer)

	ctx := context.Background()
	authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "uploader-123", OUHandle: "company-xyz"}}

	meta := &storage.FileMetadata{
		Key: "file-key-999.pdf",
	}

	if err := svc.OnUpload(ctx, meta, authCtx); err != nil {
		t.Fatalf("OnUpload failed: %v", err)
	}

	colleagueCtx := &authn.AuthContext{User: &authn.UserContext{ID: "colleague-456", OUHandle: "company-xyz"}}
	allowed, err := svc.ValidateAccess(ctx, meta.Key, colleagueCtx)
	if err != nil || !allowed {
		t.Errorf("expected company access allowed after OnUpload, got allowed=%v err=%v", allowed, err)
	}
}

func TestObjectMetadataService_RecordMetadata_UpsertEnrichment(t *testing.T) {
	db := setupTestDB(t)
	svc := NewObjectMetadataService(db, nil)
	ctx := context.Background()
	key := "upsert-enrichment-key.pdf"

	// 1. Initial upload without task_id / consignment_id
	if err := svc.RecordMetadata(ctx, key, "", "", "company-initial", "uploader-initial"); err != nil {
		t.Fatalf("initial RecordMetadata failed: %v", err)
	}

	// 2. Later workflow attachment: provide task_id and consignment_id with empty uploaded_by / company_id
	if err := svc.RecordMetadata(ctx, key, "task-enriched", "consignment-enriched", "", ""); err != nil {
		t.Fatalf("second RecordMetadata (enrichment) failed: %v", err)
	}

	var rec ObjectMetadata
	if err := db.First(&rec, "key = ?", key).Error; err != nil {
		t.Fatalf("failed to query enriched record: %v", err)
	}

	if rec.TaskID != "task-enriched" {
		t.Errorf("expected TaskID to be enriched to 'task-enriched', got %q", rec.TaskID)
	}
	if rec.ConsignmentID != "consignment-enriched" {
		t.Errorf("expected ConsignmentID to be enriched to 'consignment-enriched', got %q", rec.ConsignmentID)
	}
	if rec.CompanyID != "company-initial" {
		t.Errorf("expected CompanyID to remain preserved as 'company-initial', got %q", rec.CompanyID)
	}
	if rec.UploadedBy != "uploader-initial" {
		t.Errorf("expected UploadedBy to remain preserved as 'uploader-initial', got %q", rec.UploadedBy)
	}
}
