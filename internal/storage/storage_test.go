package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
)

// testUploadTypes and testMaxUploadBytes are the upload limits the test
// configurations set.
var testUploadTypes = []string{"application/pdf", "image/png"}

// testTokenKeyset is a valid storage.tokenKeyset, built rather than written
// out so no key literal sits in the source.
var testTokenKeyset = "test:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))

const testMaxUploadBytes = 1 << 20

// localConfig is a valid local-backend configuration whose URLs point at
// publicURL.
func localConfig(t *testing.T, publicURL string) Config {
	t.Helper()
	return Config{
		Config: corestorage.Config{
			Type: corestorage.TypeLocal,
			Local: drivers.LocalConfig{
				BaseDir:   t.TempDir(),
				PublicURL: publicURL,
				PutSecret: "secret",
			},
			PresignTTLSeconds: 900,
		},
		AllowedUploadTypes: testUploadTypes,
		MaxUploadBytes:     testMaxUploadBytes,
		TokenKeyset:        testTokenKeyset,
	}
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
	// New refuses it too, rather than replacing it with RoutePrefix.
	if _, err := New(context.Background(), cfg, nil); err == nil || !strings.Contains(err.Error(), "storage.local.routePrefix") {
		t.Errorf("routePrefix /files: New() = %v, want an error naming storage.local.routePrefix", err)
	}
}

// The backend enforces the upload limits from the configuration, not core's
// defaults (any type, 32 MiB).
func TestNew_AppliesConfiguredUploadLimits(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	for _, mime := range testUploadTypes {
		if _, err := stack.Service.Upload(ctx, "file", 10, mime); err != nil {
			t.Errorf("Upload(%s) = %v, want it accepted", mime, err)
		}
	}
	for _, mime := range []string{"application/x-msdownload", "application/vnd.ms-excel", "text/html"} {
		if _, err := stack.Service.Upload(ctx, "file", 10, mime); !errors.Is(err, corestorage.ErrContentTypeNotAllowed) {
			t.Errorf("Upload(%s) = %v, want ErrContentTypeNotAllowed", mime, err)
		}
	}

	if _, err := stack.Service.Upload(ctx, "a.pdf", testMaxUploadBytes, "application/pdf"); err != nil {
		t.Errorf("Upload at maxUploadBytes = %v, want it accepted", err)
	}
	var tooLarge *corestorage.FileTooLargeError
	if _, err := stack.Service.Upload(ctx, "a.pdf", testMaxUploadBytes+1, "application/pdf"); !errors.As(err, &tooLarge) {
		t.Errorf("Upload over maxUploadBytes = %v, want *FileTooLargeError", err)
	}
}

func TestConfigValidate_UploadLimits(t *testing.T) {
	tests := []struct {
		name    string
		types   []string
		max     int64
		wantErr string
	}{
		{name: "no types", types: nil, max: 1, wantErr: "storage.allowedUploadTypes must list"},
		{name: "empty entry", types: []string{""}, max: 1, wantErr: "storage.allowedUploadTypes"},
		{name: "no subtype", types: []string{"pdf"}, max: 1, wantErr: "storage.allowedUploadTypes"},
		{name: "parameters", types: []string{"text/plain; charset=utf-8"}, max: 1, wantErr: "storage.allowedUploadTypes"},
		{name: "upper case", types: []string{"Application/PDF"}, max: 1, wantErr: "storage.allowedUploadTypes"},
		{name: "wildcard", types: []string{"image/*"}, max: 1, wantErr: "storage.allowedUploadTypes"},
		{name: "no size", types: testUploadTypes, max: 0, wantErr: "storage.maxUploadBytes"},
		{name: "negative size", types: testUploadTypes, max: -1, wantErr: "storage.maxUploadBytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := localConfig(t, "http://localhost:8080")
			cfg.AllowedUploadTypes = tt.types
			cfg.MaxUploadBytes = tt.max

			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want an error containing %q", err, tt.wantErr)
			}
			// New refuses it the same way, instead of reaching core's panic
			// on a non-positive size.
			if _, err := New(context.Background(), cfg, nil); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("New() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}

	t.Run("valid", func(t *testing.T) {
		if err := localConfig(t, "http://localhost:8080").Validate(); err != nil {
			t.Errorf("Validate() = %v, want nil", err)
		}
	})

	// The owning service enforces its own limits in proxy mode, so they are
	// not read there.
	t.Run("proxy mode needs none", func(t *testing.T) {
		cfg := Config{
			Config: corestorage.Config{Type: TypeProxy},
			Proxy: ProxyConfig{
				Service:      "files-api",
				UploadPath:   DefaultProxyUploadPath,
				DownloadPath: DefaultProxyDownloadPath,
				DeletePath:   DefaultProxyDeletePath,
			},
			TokenKeyset: testTokenKeyset,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() = %v, want nil", err)
		}
	})
}

// Every mode issues file tokens, so every mode needs the token settings.
func TestConfigValidate_FileTokens(t *testing.T) {
	proxy := Config{Config: corestorage.Config{Type: TypeProxy}, Proxy: defaultProxyConfig()}
	backend := localConfig(t, "http://localhost:8080")

	tests := []struct {
		name    string
		keyset  string
		ttl     int
		wantErr string
		wantTTL time.Duration
	}{
		{name: "no keyset", keyset: "", wantErr: "storage.tokenKeyset is required"},
		{name: "malformed keyset", keyset: "test:not-base64!", wantErr: "storage.tokenKeyset"},
		{name: "short key", keyset: "test:" + base64.StdEncoding.EncodeToString([]byte("short")), wantErr: "storage.tokenKeyset"},
		{name: "negative TTL", keyset: testTokenKeyset, ttl: -1, wantErr: "storage.tokenTTLSeconds"},
		{name: "default TTL", keyset: testTokenKeyset, wantTTL: time.Hour},
		{name: "set TTL", keyset: testTokenKeyset, ttl: 600, wantTTL: 10 * time.Minute},
	}
	for _, tt := range tests {
		for mode, cfg := range map[string]Config{"proxy": proxy, "backend": backend} {
			t.Run(tt.name+" in "+mode+" mode", func(t *testing.T) {
				cfg.TokenKeyset, cfg.TokenTTLSeconds = tt.keyset, tt.ttl
				err := cfg.Validate()
				if tt.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
						t.Errorf("Validate() = %v, want an error containing %q", err, tt.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				if got := cfg.TokenTTL(); got != tt.wantTTL {
					t.Errorf("TokenTTL() = %v, want %v", got, tt.wantTTL)
				}
			})
		}
	}
}
