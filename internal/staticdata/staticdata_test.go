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

// newTwoManifestSet returns a set whose manifest 1 lists ports_1.json and whose manifest 2
// lists ports_2.json, both under id "ports" with no version, as a release that replaces the
// data publishes them.
func newTwoManifestSet(t *testing.T) *artifact.Set {
	t.Helper()
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"ports_1.json":     []byte(`{"data":["CMB"]}`),
		"ports_2.json":     []byte(`{"data":["CMB","HRI"]}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","path":"ports_1.json"}]}`),
		"manifest_v2.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","path":"ports_2.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// Each manifest version resolves the data it lists, so a case pinned to manifest 1 keeps
// the list it started with after manifest 2 replaces it.
func TestLoadAt_ResolvesEachManifestsOwnData(t *testing.T) {
	set := newTwoManifestSet(t)
	for v, want := range map[int]string{1: `{"data":["CMB"]}`, 2: `{"data":["CMB","HRI"]}`} {
		raw, err := LoadAt(context.Background(), set, "ports", v)
		if err != nil || string(raw) != want {
			t.Errorf("manifest %d: got %s, %v; want %s", v, raw, err, want)
		}
	}
}

// A manifest that lists the id only at explicit versions resolves its highest one.
func TestLoadAt_ExplicitVersionsResolveHighest(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"ports_1.json":     []byte(`{"data":["CMB"]}`),
		"ports_2.json":     []byte(`{"data":["CMB","HRI"]}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","version":"1","path":"ports_1.json"},{"id":"ports","kind":"static_data","version":"2","path":"ports_2.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := LoadAt(context.Background(), set, "ports", 1)
	if err != nil || string(raw) != `{"data":["CMB","HRI"]}` {
		t.Fatalf("got %s, %v; want version 2's data", raw, err)
	}
}

func TestLoadAt_UnknownManifestVersion(t *testing.T) {
	set := newTwoManifestSet(t)
	if _, err := LoadAt(context.Background(), set, "ports", 3); !errors.Is(err, artifact.ErrUnknownManifestVersion) {
		t.Fatalf("expected ErrUnknownManifestVersion, got %v", err)
	}
}

func TestLoadAt_UnknownID(t *testing.T) {
	set := newTwoManifestSet(t)
	if _, err := LoadAt(context.Background(), set, "missing", 1); !errors.Is(err, artifact.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLoadAt_InvalidArtifact(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"ports.json":       []byte(`["CMB"]`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","path":"ports.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAt(context.Background(), set, "ports", 1); err == nil {
		t.Fatal("expected a bare array to be rejected")
	}
}

// A manifest that lists the id only at an explicit version, as the manifests published so far
// do, resolves that version.
func TestLoadAt_ExplicitVersionOnly(t *testing.T) {
	set := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":["0101"]}`))

	raw, err := LoadAt(context.Background(), set, "hs-codes", 1)
	if err != nil || string(raw) != `{"data":["0101"]}` {
		t.Fatalf("got %s, %v", raw, err)
	}
}
