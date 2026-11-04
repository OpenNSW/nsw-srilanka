package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// gatedLoader serves manifests at once and holds every other file until release is closed or
// the caller's context ends. It counts the file loads and closes started on the first one.
type gatedLoader struct {
	testutil.MemLoader
	started chan struct{}
	release chan struct{}
	once    sync.Once
	loads   atomic.Int32
}

func newGatedLoader() *gatedLoader {
	return &gatedLoader{
		MemLoader: testutil.MemLoader{
			"form.json":        []byte(`{"title":"form"}`),
			"manifest_v1.json": []byte(`{"artifacts":[{"id":"form","kind":"generic_template","path":"form.json"}]}`),
		},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (l *gatedLoader) Load(ctx context.Context, path string) ([]byte, error) {
	if strings.HasPrefix(path, "manifest") {
		return l.MemLoader.Load(ctx, path)
	}
	l.loads.Add(1)
	l.once.Do(func() { close(l.started) })
	select {
	case <-l.release:
		return l.MemLoader.Load(ctx, path)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// warmArtifacts returns while the files are still loading, and the warmed file is then served
// from the cache without another load.
func TestWarmArtifactsRunsInTheBackground(t *testing.T) {
	l := newGatedLoader()
	set, err := artifact.LoadSet(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}

	stop := warmArtifacts(context.Background(), set)
	<-l.started // warmArtifacts has returned, and the warm-up is still blocked on the file
	close(l.release)
	stop()

	p := registryTemplateProvider{artifacts: set}
	if _, err := p.GetTemplate(artifact.WithManifestVersion(context.Background(), 1), "form"); err != nil {
		t.Fatal(err)
	}
	if n := l.loads.Load(); n != 1 {
		t.Fatalf("file loads = %d, want 1: the lookup after warming should hit the cache", n)
	}
}

// stop cancels a warm-up that is still loading and waits for it to end.
func TestWarmArtifactsStopCancels(t *testing.T) {
	l := newGatedLoader()
	set, err := artifact.LoadSet(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}

	stop := warmArtifacts(context.Background(), set)
	<-l.started

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return while the warm-up was loading")
	}
	stop() // a second stop, as from both a failed Build and Close, returns at once
}

// A warm-up started from a context that is later cancelled keeps running: Build's context
// does not bound the server's life.
func TestWarmArtifactsOutlivesTheCallersContext(t *testing.T) {
	l := newGatedLoader()
	set, err := artifact.LoadSet(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stop := warmArtifacts(ctx, set)
	<-l.started
	cancel()
	close(l.release)
	stop()

	p := registryTemplateProvider{artifacts: set}
	if _, err := p.GetTemplate(artifact.WithManifestVersion(context.Background(), 1), "form"); err != nil {
		t.Fatal(err)
	}
	if n := l.loads.Load(); n != 1 {
		t.Fatalf("file loads = %d, want 1: the warm-up should have finished despite the cancelled context", n)
	}
}

// With the file cache off, warming fetches nothing and stop returns at once.
func TestWarmArtifactsWithoutAFileCache(t *testing.T) {
	l := newGatedLoader()
	set, err := artifact.LoadSet(context.Background(), l, artifact.WithFileCacheBytes(0))
	if err != nil {
		t.Fatal(err)
	}

	warmArtifacts(context.Background(), set)()
	if n := l.loads.Load(); n != 0 {
		t.Fatalf("file loads = %d, want 0 with the file cache off", n)
	}
}
