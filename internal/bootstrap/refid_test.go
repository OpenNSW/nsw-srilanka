package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/OpenNSW/core/refid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// With no issuers there is nothing to generate from, so the database is never
// touched (a nil db proves it) and every Generate fails loudly.
func TestInitRefIDs_NotConfigured(t *testing.T) {
	reg, err := initRefIDs(refid.Config{}, nil)
	if err != nil {
		t.Fatalf("initRefIDs: unexpected error: %v", err)
	}
	_, err = reg.Generate(context.Background(), "TNSW", "consignment_ref", nil)
	if !errors.Is(err, refid.ErrUnknownIssuer) {
		t.Fatalf("expected ErrUnknownIssuer from the disabled registry, got %v", err)
	}
}

// A malformed format stops the boot rather than failing the first task that
// uses it. The db is never dialled: stores are built without a round trip.
func TestInitRefIDs_MalformedConfigFails(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable"}),
		&gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	cfg := refid.Config{Issuers: []refid.IssuerConfig{{
		Issuer: "TNSW",
		Formats: []refid.FormatConfig{{
			IDType: "consignment_ref",
			// A sequence segment needs a scopeKey.
			Segments: []refid.SegmentConfig{{Type: refid.SegmentTypeSequence, Sequence: &refid.SequenceSegmentConfig{Padding: 6}}},
		}},
	}}}

	_, err = initRefIDs(cfg, db)
	if err == nil || !strings.Contains(err.Error(), "invalid refid config") {
		t.Fatalf("expected an invalid refid config error, got %v", err)
	}
}
