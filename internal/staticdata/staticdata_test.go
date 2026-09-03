package staticdata

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
)

func newTestRegistry(t *testing.T, id, version, path string, body []byte) *artifact.Registry {
	t.Helper()
	loader := testutil.MemLoader{path: body}
	reg := artifact.NewRegistry(loader)
	reg.RegisterArtifact(id, Kind, version, path)
	return reg
}

func TestLoad_Success(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":["0101","0102"]}`))

	raw, err := Load(context.Background(), reg, "hs-codes", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(raw) != `{"data":["0101","0102"]}` {
		t.Fatalf("unexpected body: %s", raw)
	}
}

func TestLoad_UnknownID(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":[]}`))

	_, err := Load(context.Background(), reg, "missing", "1.0.0")
	if !errors.Is(err, artifact.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLoad_UnknownVersion(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":[]}`))

	_, err := Load(context.Background(), reg, "hs-codes", "2.0.0")
	if !errors.Is(err, artifact.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`not json`))

	if _, err := Load(context.Background(), reg, "hs-codes", "1.0.0"); err == nil {
		t.Fatal("expected an error for invalid json, got nil")
	}
}

func TestLoad_MissingDataField(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"codes":["0101"]}`))

	if _, err := Load(context.Background(), reg, "hs-codes", "1.0.0"); err == nil {
		t.Fatal(`expected an error for a missing "data" field, got nil`)
	}
}

func TestLoad_BareArray(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`["0101","0102"]`))

	if _, err := Load(context.Background(), reg, "hs-codes", "1.0.0"); err == nil {
		t.Fatal("expected an error for a bare top-level array, got nil")
	}
}

func TestLoad_DataFieldNotAnArray(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":"0101"}`))

	if _, err := Load(context.Background(), reg, "hs-codes", "1.0.0"); err == nil {
		t.Fatal(`expected an error when "data" is not an array, got nil`)
	}
}
