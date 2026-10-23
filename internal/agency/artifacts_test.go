package agency

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
)

// newSet builds an artifact set from files: manifests[i] lists the rows of manifest
// version i+1, and the last one is current.
func newSet(t *testing.T, files map[string]string, manifests ...[]artifact.ManifestRow) *artifact.Set {
	t.Helper()
	m := testutil.MemLoader{}
	for path, body := range files {
		m[path] = []byte(body)
	}
	for i, rows := range manifests {
		raw, err := json.Marshal(artifact.ManifestConfig{Artifacts: rows})
		if err != nil {
			t.Fatal(err)
		}
		m[artifact.ManifestPath(i+1)] = raw
	}
	set, err := artifact.LoadSet(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func row(id string, kind artifact.Kind, path string) artifact.ManifestRow {
	return artifact.ManifestRow{ID: id, Kind: kind, Path: path}
}
