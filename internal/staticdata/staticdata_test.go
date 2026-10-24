package staticdata

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
)

// newTestRegistry returns a one-manifest artifact set listing id at version.
func newTestRegistry(t *testing.T, id, version, path string, body []byte) *artifact.Set {
	t.Helper()
	manifest, err := json.Marshal(artifact.ManifestConfig{Artifacts: []artifact.ManifestRow{{ID: id, Kind: Kind, Version: version, Path: path}}})
	if err != nil {
		t.Fatal(err)
	}
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{path: body, artifact.ManifestPath(1): manifest})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// A version an older manifest listed still resolves after a newer manifest stops listing
// it: a form from a flow pinned to the older manifest still asks for it.
func TestLoad_VersionListedOnlyByAnOlderManifest(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"ports_1.json":     []byte(`{"data":["CMB"]}`),
		"ports_2.json":     []byte(`{"data":["CMB","HRI"]}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","version":"1","path":"ports_1.json"}]}`),
		"manifest_v2.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","version":"2","path":"ports_2.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	for version, want := range map[string]string{"1": `{"data":["CMB"]}`, "2": `{"data":["CMB","HRI"]}`} {
		raw, err := Load(context.Background(), set, "ports", version)
		if err != nil || string(raw) != want {
			t.Errorf("version %s: got %s, %v; want %s", version, raw, err, want)
		}
	}
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

func TestLoad_DataFieldIsNull(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":null}`))

	if _, err := Load(context.Background(), reg, "hs-codes", "1.0.0"); err == nil {
		t.Fatal(`expected an error when "data" is null, got nil`)
	}
}
