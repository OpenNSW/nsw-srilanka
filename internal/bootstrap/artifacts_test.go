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

func TestArtifactChecksCoverEveryKindThisBinaryReads(t *testing.T) {
	checks := artifactChecks()
	for _, kind := range []artifact.Kind{"workflow", "task_template", "subtask_template", "generic_template", "task_config", "static_data"} {
		if checks[kind] == nil {
			t.Errorf("no check for %s", kind)
		}
	}
}

// initArtifacts validates the current manifest unless told not to, and honours the pin.
func TestInitArtifacts(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := writeFile(filepath.Join(dir, name), body); err != nil {
			t.Fatal(err)
		}
	}
	write("flow.json", `{"id":"flow","nodes":[{"id":"t","type":"TASK","task_template_id":"apply"}]}`)
	write("manifest_v1.json", `{"artifacts":[{"id":"flow","kind":"workflow","path":"flow.json"},{"id":"apply","kind":"subtask_template","path":"apply.json"}]}`)
	write("apply.json", `{"id":"apply","task_type":"USER_INPUT"}`)
	// v2 lists a workflow whose task template it does not list.
	write("manifest_v2.json", `{"artifacts":[{"id":"flow","kind":"workflow","path":"flow.json"}]}`)

	cfg := &config.Config{ArtifactLoader: loaders.Config{Type: "local", Local: local.Config{Root: dir}}}

	if _, err := initArtifacts(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "artifact manifest 2 is invalid") {
		t.Fatalf("err = %v, want manifest 2 rejected", err)
	}

	cfg.Artifacts.ManifestVersion = 1
	set, err := initArtifacts(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pinned to the good v1: %v", err)
	}
	if set.Current() != 1 {
		t.Errorf("current = %d, want the pinned 1", set.Current())
	}

	cfg.Artifacts = config.ArtifactsConfig{SkipStartupValidation: true}
	if set, err = initArtifacts(context.Background(), cfg); err != nil || set.Current() != 2 {
		t.Fatalf("skipping validation: current %v, err %v; want 2, nil", set, err)
	}
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

// A step template's notification must name a template the manifest lists.
func TestArtifactChecksReportANotificationTemplate(t *testing.T) {
	check := artifactChecks()["subtask_template"]
	refs, err := check([]byte(`{"id":"s","task_type":"USER_INPUT","extensions":[
		{"id":"notification","phase":"POST_RESUME","properties":{"channel":"EMAIL","template_id":"approved-email"}},
		{"id":"other","phase":"PRE_RESUME","properties":{"template_id":"ignored"}}]}`))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != "approved-email" || refs[0].Kinds[0] != "generic_template" {
		t.Fatalf("refs = %+v, want approved-email", refs)
	}
	if _, err := check([]byte(`{"task_type":"USER_INPUT"}`)); err == nil {
		t.Error("expected core's check to reject a step template without an id")
	}
}
