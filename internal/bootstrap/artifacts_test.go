package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/artifact/testutil"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

func templateSet(t *testing.T) *artifact.Set {
	t.Helper()
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"form_v1.json":     []byte(`{"title":"v1"}`),
		"form_v2.json":     []byte(`{"title":"v2"}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"form","kind":"generic_template","path":"form_v1.json"}]}`),
		"manifest_v2.json": []byte(`{"artifacts":[{"id":"form","kind":"generic_template","path":"form_v2.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// The template provider resolves through the version on the context, never the current one.
func TestRegistryTemplateProviderUsesTheContextVersion(t *testing.T) {
	p := registryTemplateProvider{artifacts: templateSet(t)}

	raw, err := p.GetTemplate(artifact.WithManifestVersion(context.Background(), 1), "form")
	if err != nil || string(raw) != `{"title":"v1"}` {
		t.Fatalf("v1: got %s, %v; want the v1 form", raw, err)
	}
	if _, err := p.GetTemplate(context.Background(), "form"); err == nil || !strings.Contains(err.Error(), "no artifact manifest version") {
		t.Fatalf("without a version: err = %v, want a missing-version error", err)
	}
	if _, err := p.GetTemplate(artifact.WithManifestVersion(context.Background(), 0), "form"); err == nil {
		t.Fatal("version 0: want an error")
	}
}

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
