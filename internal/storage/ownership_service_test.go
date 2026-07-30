package storage

import (
	"context"
	"testing"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockConsignmentGetter struct {
	traderID string
	chaID    string
	err      error
}

func (m *mockConsignmentGetter) GetOwnership(ctx context.Context, consignmentID string) (string, string, error) {
	return m.traderID, m.chaID, m.err
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite test DB: %v", err)
	}
	if err := db.AutoMigrate(&StorageObjectOwnership{}); err != nil {
		t.Fatalf("failed to migrate test DB: %v", err)
	}
	return db
}

func TestOwnershipService_ValidateAccess(t *testing.T) {
	db := setupTestDB(t)
	getter := &mockConsignmentGetter{traderID: "company-trader-1", chaID: "company-cha-1"}
	svc := NewOwnershipService(db, getter)

	ctx := context.Background()
	key := "00000000-0000-0000-0000-000000000001.pdf"

	if err := svc.RecordOwnership(ctx, key, "task-1", "c-1", "user-1"); err != nil {
		t.Fatalf("RecordOwnership failed: %v", err)
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

	t.Run("uploader allowed", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-1", OUHandle: "company-other"}}
		allowed, err := svc.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Error("expected uploader user-1 to be allowed")
		}
	})

	t.Run("trader company allowed", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-2", OUHandle: "company-trader-1"}}
		allowed, err := svc.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Error("expected matching trader company to be allowed")
		}
	})

	t.Run("mismatched trader company denied", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-3", OUHandle: "company-trader-2"}}
		allowed, err := svc.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("expected mismatched trader company to be denied")
		}
	})

	t.Run("agency officer allowed", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "officer-1", Roles: []string{"AGENCY_OFFICER"}}}
		allowed, err := svc.ValidateAccess(ctx, key, authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Error("expected agency officer to be allowed")
		}
	})

	t.Run("unmapped key denied", func(t *testing.T) {
		authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "user-1", OUHandle: "company-trader-1"}}
		allowed, err := svc.ValidateAccess(ctx, "unmapped-key.pdf", authCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("expected unmapped key to be denied access")
		}
	})
}

func TestOwnershipService_OnUpload(t *testing.T) {
	db := setupTestDB(t)
	getter := &mockConsignmentGetter{traderID: "company-trader-1", chaID: "company-cha-1"}
	svc := NewOwnershipService(db, getter)

	ctx := context.Background()
	authCtx := &authn.AuthContext{User: &authn.UserContext{ID: "uploader-123", OUHandle: "company-trader-1"}}

	meta := &storage.FileMetadata{
		Key: "file-key-999.pdf",
		Ownership: map[string]any{
			"task_id":        "task-999",
			"consignment_id": "c-999",
		},
	}

	if err := svc.OnUpload(ctx, meta, authCtx); err != nil {
		t.Fatalf("OnUpload failed: %v", err)
	}

	allowed, err := svc.ValidateAccess(ctx, meta.Key, authCtx)
	if err != nil || !allowed {
		t.Errorf("expected access allowed after OnUpload, got allowed=%v err=%v", allowed, err)
	}

	otherCtx := &authn.AuthContext{User: &authn.UserContext{ID: "other-user", OUHandle: "company-trader-99"}}
	allowedOther, err := svc.ValidateAccess(ctx, meta.Key, otherCtx)
	if err != nil || allowedOther {
		t.Errorf("expected access denied for mismatched trader, got allowed=%v err=%v", allowedOther, err)
	}
}
