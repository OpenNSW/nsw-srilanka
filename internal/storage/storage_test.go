package storage

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
)

// localConfig is a valid local-backend configuration whose URLs point at
// publicURL.
func localConfig(t *testing.T, publicURL string) Config {
	t.Helper()
	return Config{Config: corestorage.Config{
		Type: corestorage.TypeLocal,
		Local: drivers.LocalConfig{
			BaseDir:   t.TempDir(),
			PublicURL: publicURL,
			PutSecret: "secret",
		},
		PresignTTLSeconds: 900,
	}}
}

// The local backend's upload URLs point under RoutePrefix, and the content
// routes the stack mounts serve them there.
func TestNew_LocalContentUnderRoutePrefix(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	stack, err := New(context.Background(), localConfig(t, srv.URL), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stack.LocalContent.RegisterRoutes(mux)

	content := []byte("%PDF-1.7")
	meta, err := stack.Service.Upload(context.Background(), "a.pdf", int64(len(content)), "application/pdf")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if want := srv.URL + RoutePrefix + "/" + meta.Key + "/content?"; !strings.HasPrefix(meta.UploadURL, want) {
		t.Fatalf("UploadURL = %s, want it under %s", meta.UploadURL, want)
	}

	req, err := http.NewRequest(http.MethodPut, meta.UploadURL, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("PUT %s = %d, want %d", meta.UploadURL, resp.StatusCode, http.StatusNoContent)
	}
}

func TestConfigValidate_LocalRoutePrefix(t *testing.T) {
	for _, prefix := range []string{"", RoutePrefix} {
		cfg := localConfig(t, "http://localhost:8080")
		cfg.Local.RoutePrefix = prefix
		if err := cfg.Validate(); err != nil {
			t.Errorf("routePrefix %q: unexpected error: %v", prefix, err)
		}
	}

	cfg := localConfig(t, "http://localhost:8080")
	cfg.Local.RoutePrefix = "/files"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "storage.local.routePrefix") {
		t.Errorf("routePrefix /files: Validate() = %v, want an error naming storage.local.routePrefix", err)
	}
}
