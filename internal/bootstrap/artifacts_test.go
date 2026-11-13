package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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

// registryStaticData.Title names a code by its row in the static data the
// manifest version it is given lists, and reports a malformed artifact as
// artifact.ErrInvalid, so the generator fails the step for an admin instead of
// retrying it.
func TestRegistryStaticData_Title(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"ports_1.json": []byte(`{"data":[{"const":"LKCMB","title":"COLOMBO"}]}`),
		"ports_2.json": []byte(`{"data":[{"const":"LKCMB","title":"PORT OF COLOMBO"}]}`),
		"notitle.json": []byte(`{"data":[{"const":"LKCMB"}]}`),
		"manifest_v1.json": []byte(`{"artifacts":[
			{"id":"ports","kind":"static_data","version":"1","path":"ports_1.json"},
			{"id":"notitle","kind":"static_data","version":"1","path":"notitle.json"}]}`),
		"manifest_v2.json": []byte(`{"artifacts":[
			{"id":"ports","kind":"static_data","version":"2","path":"ports_2.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := registryStaticData{artifacts: set}

	cases := map[string]struct {
		manifest  int
		id, value string
		wantTitle string
		wantFound bool
		wantErr   error
	}{
		"a known code":                     {1, "ports", "LKCMB", "COLOMBO", true, nil},
		"the static data of that manifest": {2, "ports", "LKCMB", "PORT OF COLOMBO", true, nil},
		"an unknown code":                  {1, "ports", "XXABC", "", false, nil},
		"an id the manifest doesn't list":  {2, "notitle", "LKCMB", "", false, artifact.ErrNotFound},
		"an unpublished manifest version":  {3, "ports", "LKCMB", "", false, artifact.ErrUnknownManifestVersion},
		"a row without a title":            {1, "notitle", "LKCMB", "", false, artifact.ErrInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			title, found, err := s.Title(context.Background(), tc.manifest, tc.id, tc.value)
			if !errors.Is(err, tc.wantErr) || title != tc.wantTitle || found != tc.wantFound {
				t.Fatalf("got %q, %v, %v; want %q, %v, %v", title, found, err, tc.wantTitle, tc.wantFound, tc.wantErr)
			}
		})
	}

	// The manifest version passed in decides, not one on the context.
	ctx := artifact.WithManifestVersion(context.Background(), 1)
	if title, _, err := s.Title(ctx, 2, "ports", "LKCMB"); err != nil || title != "PORT OF COLOMBO" {
		t.Fatalf("with version 1 on the context: got %q, %v; want manifest 2's title", title, err)
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

// gatedLoader serves manifests at once and holds every other file until release is closed or,
// unless ignoreCancel is set, the caller's context ends. Once release is closed it always
// serves the file, even if the context has ended too, so a test that releases the load before
// calling stop sees it cached. It counts the file loads, keeps the context of the first one in
// loadCtx and then closes started.
type gatedLoader struct {
	testutil.MemLoader
	ignoreCancel bool
	started      chan struct{}
	release      chan struct{}
	loadCtx      context.Context
	once         sync.Once
	loads        atomic.Int32
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
	l.once.Do(func() {
		l.loadCtx = ctx
		close(l.started)
	})
	done := ctx.Done()
	if l.ignoreCancel {
		done = nil
	}
	select {
	case <-l.release:
		return l.MemLoader.Load(ctx, path)
	case <-done:
		select {
		case <-l.release:
			return l.MemLoader.Load(ctx, path)
		default:
			return nil, ctx.Err()
		}
	}
}

// waitFor fails the test if ch isn't closed within five seconds.
func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// captureLogs sends the default logger to a buffer for the rest of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &logs
}

// warmArtifacts returns while the files are still loading, and the warmed file is then served
// from the cache without another load.
func TestWarmArtifactsRunsInTheBackground(t *testing.T) {
	l := newGatedLoader()
	set, err := artifact.LoadSet(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}

	stop := warmArtifacts(context.Background(), set, warmStopTimeout)
	waitFor(t, l.started, "the warm-up to load the file") // warmArtifacts has returned, and the warm-up is blocked on the file
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

// stop cancels a warm-up that is still loading, waits for it to end, and the cut-short warm-up
// is logged as a warning.
func TestWarmArtifactsStopCancels(t *testing.T) {
	logs := captureLogs(t)
	l := newGatedLoader()
	set, err := artifact.LoadSet(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}

	stop := warmArtifacts(context.Background(), set, warmStopTimeout)
	waitFor(t, l.started, "the warm-up to load the file")

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	waitFor(t, stopped, "stop to return while the warm-up was loading")
	if out := logs.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "failed to warm the artifact file cache") {
		t.Fatalf("logs = %q, want a warning for the cut-short warm-up", out)
	}
	stop() // calling stop twice is safe, and the second call returns at once
}

// stop gives up after its timeout when a load ignores the cancel, and logs that it did.
func TestWarmArtifactsStopTimesOut(t *testing.T) {
	logs := captureLogs(t)
	l := newGatedLoader()
	l.ignoreCancel = true
	defer close(l.release) // lets the abandoned warm-up finish after the test
	set, err := artifact.LoadSet(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}

	stop := warmArtifacts(context.Background(), set, 50*time.Millisecond)
	waitFor(t, l.started, "the warm-up to load the file")

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	waitFor(t, stopped, "stop to give up on a load that ignores the cancel")
	if out := logs.String(); !strings.Contains(out, "did not stop in time") {
		t.Fatalf("logs = %q, want a warning that the warm-up did not stop in time", out)
	}
	stop() // a second call doesn't wait out the timeout again
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
	stop := warmArtifacts(ctx, set, warmStopTimeout)
	waitFor(t, l.started, "the warm-up to load the file")
	cancel()
	// A cancel reaches every derived context before cancel returns, so this fails at once if
	// the warm-up's context were derived from ctx.
	if err := l.loadCtx.Err(); err != nil {
		t.Fatalf("the load's context ended with the caller's: %v", err)
	}
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

	warmArtifacts(context.Background(), set, warmStopTimeout)()
	if n := l.loads.Load(); n != 0 {
		t.Fatalf("file loads = %d, want 0 with the file cache off", n)
	}
}

// artifacts.fileCacheBytes of 0 keeps core's default cache and a negative value turns it off:
// a file changed on disk after its first lookup is served from the cache only when it is on.
func TestInitArtifactsFileCacheBytes(t *testing.T) {
	for _, tc := range []struct {
		bytes  int
		cached bool
	}{{0, true}, {-1, false}} {
		dir := t.TempDir()
		write := func(name, body string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("form.json", `{"title":"before"}`)
		write("manifest_v1.json", `{"artifacts":[{"id":"form","kind":"generic_template","path":"form.json"}]}`)

		cfg := &config.Config{ArtifactLoader: loaders.Config{Type: "local", Local: local.Config{Root: dir}}}
		cfg.Artifacts.FileCacheBytes = tc.bytes
		set, err := initArtifacts(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}

		p := registryTemplateProvider{artifacts: set}
		ctx := artifact.WithManifestVersion(context.Background(), 1)
		if _, err := p.GetTemplate(ctx, "form"); err != nil {
			t.Fatal(err)
		}
		write("form.json", `{"title":"after"}`)
		raw, err := p.GetTemplate(ctx, "form")
		if err != nil {
			t.Fatal(err)
		}
		want := `{"title":"after"}`
		if tc.cached {
			want = `{"title":"before"}`
		}
		if string(raw) != want {
			t.Errorf("fileCacheBytes %d: got %s, want %s", tc.bytes, raw, want)
		}
	}
}
