package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/local"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

// initArtifacts makes the highest manifest current unless a pin names another, and loads
// a source with only manifest.json (an artifacts repository checkout) as version 1.
func TestInitArtifacts(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("flow.json", `{"id":"flow","nodes":[]}`)
	write("manifest.json", `{"artifacts":[{"id":"flow","kind":"workflow","path":"flow.json"}]}`)

	cfg := &config.Config{ArtifactLoader: loaders.Config{Type: "local", Local: local.Config{Root: dir}}}
	set, err := initArtifacts(context.Background(), cfg)
	if err != nil || set.Current() != 1 {
		t.Fatalf("only manifest.json: current %v, err %v; want 1, nil", set, err)
	}

	write("manifest_v1.json", `{"artifacts":[{"id":"flow","kind":"workflow","path":"flow.json"}]}`)
	write("manifest_v2.json", `{"artifacts":[{"id":"flow","kind":"workflow","path":"flow.json"}]}`)
	if set, err = initArtifacts(context.Background(), cfg); err != nil || set.Current() != 2 {
		t.Fatalf("versioned manifests: current %v, err %v; want 2, nil", set, err)
	}
	cfg.Artifacts.ManifestVersion = 1
	if set, err = initArtifacts(context.Background(), cfg); err != nil || set.Current() != 1 {
		t.Fatalf("pinned to 1: current %v, err %v; want 1, nil", set, err)
	}
	cfg.Artifacts.ManifestVersion = 3
	if _, err := initArtifacts(context.Background(), cfg); err == nil {
		t.Fatal("a pin above the highest published version: want an error")
	}
}
