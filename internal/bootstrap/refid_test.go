package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenNSW/core/refid"
	"gopkg.in/yaml.v3"
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

// The refid sections of the committed configs compile, and generate the IDs
// the artifacts that use them expect.
func TestCommittedRefIDFormats(t *testing.T) {
	yy := time.Now().UTC().Format("06")
	cda := []string{"configs/agency/cda/config.yaml", "configs/agency/cda/config.docker.yaml"}
	for _, tc := range []struct {
		name    string
		files   []string
		issuer  string
		idType  string
		params  map[string]string
		want    string
		wantErr error
	}{
		{
			name: "application ref", files: cda, issuer: "CDA", idType: "pqc_application_ref",
			params: map[string]string{"certType": "PS"}, want: "PQC/PS/0001/" + yy + "/T",
		},
		{
			name: "unknown cert type", files: cda, issuer: "CDA", idType: "pqc_application_ref",
			params: map[string]string{"certType": "X"}, wantErr: refid.ErrInvalidParam,
		},
		{
			name: "certificate serial", files: cda, issuer: "CDA", idType: "pqc_certificate_serial",
			want: "0001/" + yy,
		},
		{
			name: "physical certificate", files: cda, issuer: "CDA", idType: "pqc_physical_certificate",
			params: map[string]string{"serial": "1205/26"}, want: "PQC/P/1205/26",
		},
		{
			name: "salmonella certificate", files: cda, issuer: "CDA", idType: "pqc_salmonella_certificate",
			params: map[string]string{"serial": "1205/26"}, want: "PQC/S/1205/26",
		},
	} {
		for _, file := range tc.files {
			t.Run(tc.name+" in "+file, func(t *testing.T) {
				got, err := committedRefIDs(t, file).Generate(context.Background(), tc.issuer, tc.idType, tc.params)
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("Generate(%q, %q, %v): got (%q, %v), want %v", tc.issuer, tc.idType, tc.params, got, err, tc.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("Generate(%q, %q, %v): %v", tc.issuer, tc.idType, tc.params, err)
				}
				if got != tc.want {
					t.Errorf("Generate(%q, %q, %v) = %q, want %q", tc.issuer, tc.idType, tc.params, got, tc.want)
				}
			})
		}
	}
}

// committedRefIDs compiles the refid section of a committed config file,
// backed by an in-memory store. Only that section is decoded, so the file's
// placeholders elsewhere need no env vars.
func committedRefIDs(t *testing.T, file string) refid.Registry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var doc struct {
		RefID refid.Config `yaml:"refid"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	reg, err := refid.NewRegistry(doc.RefID, refid.WithSequenceStore(memSequences{}))
	if err != nil {
		t.Fatalf("%s: invalid refid config: %v", file, err)
	}
	return reg
}

// memSequences is an in-memory refid.SequenceStore: one counter per scope key.
type memSequences map[string]int64

func (m memSequences) Next(_ context.Context, scopeKey string, max int64) (int64, error) {
	if m[scopeKey] >= max {
		return 0, refid.ErrCounterOverflow
	}
	m[scopeKey]++
	return m[scopeKey], nil
}
