package database

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenNSW/core/database"
)

func TestOpen_RejectsNonPostgresDriver(t *testing.T) {
	cfg := database.Config{
		Driver: database.SQLite,
		SQLite: &database.SQLiteConfig{Path: ":memory:"},
	}

	_, err := Open(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "unsupported database driver") {
		t.Fatalf("expected unsupported driver error, got: %v", err)
	}
}

func TestClose_NilDB(t *testing.T) {
	if err := Close(nil); err != nil {
		t.Fatalf("Close(nil) unexpected error: %v", err)
	}
}

func TestHealthCheck_NilDB(t *testing.T) {
	if err := HealthCheck(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil db, got nil")
	}
}
