package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
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
	cda := []committedConfig{
		{file: "configs/agency/cda/config.yaml"},
		{file: "configs/agency/cda/config.docker.yaml"},
	}
	tnsw := []committedConfig{
		{file: "configs/config.example.yaml"},
		{file: "configs/config.docker.example.yaml"},
		{file: "deployments/helm/values-example.yaml", keys: []string{"backend", "config"}},
	}
	for _, tc := range []struct {
		name    string
		configs []committedConfig
		issuer  string
		idType  string
		params  map[string]string
		want    string
		pattern string // when set, the ID must match it; otherwise it must equal want
		wantErr error
	}{
		{
			name: "application ref", configs: cda, issuer: "CDA", idType: "pqc_application_ref",
			params: map[string]string{"certType": "PS"}, want: "PQC/PS/0001/" + yy + "/T",
		},
		{
			name: "unknown cert type", configs: cda, issuer: "CDA", idType: "pqc_application_ref",
			params: map[string]string{"certType": "X"}, wantErr: refid.ErrInvalidParam,
		},
		{
			name: "certificate serial", configs: cda, issuer: "CDA", idType: "pqc_certificate_serial",
			want: "0001/" + yy,
		},
		{
			name: "physical certificate", configs: cda, issuer: "CDA", idType: "pqc_physical_certificate",
			params: map[string]string{"serial": "1205/26"}, want: "PQC/P/1205/26",
		},
		{
			name: "salmonella certificate", configs: cda, issuer: "CDA", idType: "pqc_salmonella_certificate",
			params: map[string]string{"serial": "1205/26"}, want: "PQC/S/1205/26",
		},
		{
			name: "default payment ref", configs: tnsw, issuer: "TNSW", idType: "payment_ref",
			pattern: `^TNSW[A-Z0-9]{8}$`,
		},
		{
			name: "fee payment ref", configs: tnsw, issuer: "CDA", idType: "fee_payment_ref",
			params: map[string]string{"exporterId": "0001", "mainCategory": "01", "subCategory": "00002"}, want: "000101000020001",
		},
		{
			name: "fee payment ref needs a 4-digit exporter", configs: tnsw, issuer: "CDA", idType: "fee_payment_ref",
			params:  map[string]string{"exporterId": "CDA-EXP-000001", "mainCategory": "01", "subCategory": "00002"},
			wantErr: refid.ErrInvalidParam,
		},
		{
			name: "fee payment ref needs a 5-digit sub category", configs: tnsw, issuer: "CDA", idType: "fee_payment_ref",
			params:  map[string]string{"exporterId": "0001", "mainCategory": "01", "subCategory": "2"},
			wantErr: refid.ErrInvalidParam,
		},
	} {
		for _, c := range tc.configs {
			t.Run(tc.name+" in "+c.file, func(t *testing.T) {
				got, err := c.registry(t).Generate(context.Background(), tc.issuer, tc.idType, tc.params)
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("Generate(%q, %q, %v): got (%q, %v), want %v", tc.issuer, tc.idType, tc.params, got, err, tc.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("Generate(%q, %q, %v): %v", tc.issuer, tc.idType, tc.params, err)
				}
				if tc.pattern != "" {
					if !regexp.MustCompile(tc.pattern).MatchString(got) {
						t.Errorf("Generate(%q, %q, %v) = %q, want a match for %s", tc.issuer, tc.idType, tc.params, got, tc.pattern)
					}
					return
				}
				if got != tc.want {
					t.Errorf("Generate(%q, %q, %v) = %q, want %q", tc.issuer, tc.idType, tc.params, got, tc.want)
				}
			})
		}
	}
}

// committedConfig is a committed file holding a refid section, under keys.
type committedConfig struct {
	file string
	keys []string
}

// registry compiles the refid section, backed by an in-memory store. Only
// that section is decoded, so the file's placeholders elsewhere need no env
// vars.
func (c committedConfig) registry(t *testing.T) refid.Registry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", c.file))
	if err != nil {
		t.Fatalf("read %s: %v", c.file, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", c.file, err)
	}
	node := doc.Content[0]
	for _, key := range append(c.keys, "refid") {
		node = mappingValue(node, key)
		if node == nil {
			t.Fatalf("%s has no %s", c.file, strings.Join(append(c.keys, "refid"), "."))
		}
	}
	var cfg refid.Config
	if err := node.Decode(&cfg); err != nil {
		t.Fatalf("decode %s refid: %v", c.file, err)
	}
	reg, err := refid.NewRegistry(cfg, refid.WithSequenceStore(memSequences{}), refid.WithRandomStore(memRandoms{}))
	if err != nil {
		t.Fatalf("%s: invalid refid config: %v", c.file, err)
	}
	return reg
}

// mappingValue returns the value under key in a YAML mapping node, or nil.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
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

// memRandoms is an in-memory refid.RandomStore: the values reserved per scope key.
type memRandoms map[string]bool

func (m memRandoms) Reserve(_ context.Context, scopeKey, value string) error {
	if m[scopeKey+"|"+value] {
		return refid.ErrRandomCollision
	}
	m[scopeKey+"|"+value] = true
	return nil
}

// A CDA fee's payment reference counts per exporter and fee: the same exporter
// and fee take the next number, another exporter starts its own count.
func TestCommittedFeePaymentRefCountsPerExporterAndFee(t *testing.T) {
	reg := committedConfig{file: "configs/config.example.yaml"}.registry(t)
	generate := func(exporterID, subCategory string) string {
		t.Helper()
		id, err := reg.Generate(context.Background(), "CDA", "fee_payment_ref",
			map[string]string{"exporterId": exporterID, "mainCategory": "01", "subCategory": subCategory})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return id
	}
	for _, step := range []struct{ exporterID, subCategory, want string }{
		{"0002", "00002", "000201000020001"},
		{"0002", "00002", "000201000020002"},
		{"0003", "00002", "000301000020001"},
		{"0002", "00001", "000201000010001"},
	} {
		if got := generate(step.exporterID, step.subCategory); got != step.want {
			t.Errorf("exporter %s, sub category %s: got %q, want %q", step.exporterID, step.subCategory, got, step.want)
		}
	}
}
