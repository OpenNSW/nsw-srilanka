package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coreauthn "github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/remote"
	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
)

const ownerToken = "proxy-service-token"

// newOwningService stands up the service that owns the files: core/storage's
// HTTPHandler over a local-disk backend, mounted at the default proxy paths.
// Its API routes accept only the bearer token (standing in for
// service-to-service auth); the presigned content routes must arrive without
// it.
func newOwningService(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if strings.HasSuffix(r.URL.Path, "/content") {
			if authz != "" {
				t.Errorf("%s %s: presigned URL fetched with API credentials", r.Method, r.URL.Path)
			}
			mux.ServeHTTP(w, r)
			return
		}
		if authz != "Bearer "+ownerToken {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
			return
		}
		ctx := context.WithValue(r.Context(), coreauthn.AuthContextKey, &coreauthn.AuthContext{
			User: &coreauthn.UserContext{ID: "proxying-service"},
		})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)

	driver, err := drivers.NewLocalFSDriver(t.TempDir(), srv.URL, "owner-secret", 15*time.Minute)
	if err != nil {
		t.Fatalf("NewLocalFSDriver: %v", err)
	}
	// The owner applies the same upload policy this deployment does, so a
	// rejection it relays is a real one.
	h := corestorage.NewHTTPHandler(corestorage.NewService(driver, corestorage.WithAllowedUploadTypes(testUploadTypes...)))
	mux.HandleFunc(UploadRoute, h.Upload)
	mux.HandleFunc(DownloadRoute, h.Download)
	mux.HandleFunc(DeleteRoute, h.Delete)
	corestorage.NewLocalContentHandler(driver).RegisterRoutes(mux)
	return srv
}

// newRegistry registers the owning service as "files-api".
func newRegistry(t *testing.T, url, token string) *remote.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "services.json")
	registry := fmt.Sprintf(`{"version":"1","services":[{"id":"files-api","url":%q,`+
		`"auth":{"type":"bearer","options":{"token":%q}}}]}`, url, token)
	if err := os.WriteFile(path, []byte(registry), 0o600); err != nil {
		t.Fatalf("write services.json: %v", err)
	}
	m := remote.NewManager()
	if err := m.LoadServices(path); err != nil {
		t.Fatalf("LoadServices: %v", err)
	}
	return m
}

func defaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		Service:      "files-api",
		UploadPath:   DefaultProxyUploadPath,
		DownloadPath: DefaultProxyDownloadPath,
		DeletePath:   DefaultProxyDeletePath,
	}
}

func newProxyStack(t *testing.T, token string) *Stack {
	t.Helper()
	owner := newOwningService(t)
	stack, err := New(context.Background(), Config{
		Config: corestorage.Config{Type: TypeProxy},
		Proxy:  defaultProxyConfig(),
	}, newRegistry(t, owner.URL, token))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return stack
}

// TestProxy_RoundTrip uploads through the key the owning service allocates,
// downloads the file back, and deletes it.
func TestProxy_RoundTrip(t *testing.T) {
	stack := newProxyStack(t, ownerToken)
	svc := stack.Service
	ctx := context.Background()

	content := []byte("%PDF-1.4 proxy round trip")
	meta, err := svc.Upload(ctx, "passport.pdf", int64(len(content)), "application/pdf")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if meta.Key == "" || !strings.HasSuffix(meta.Key, ".pdf") || meta.Name != "passport.pdf" {
		t.Fatalf("Upload metadata = %+v, want an owner-allocated .pdf key", meta)
	}

	// The client PUTs straight to the upload URL the owning service issued.
	req, _ := http.NewRequest(http.MethodPut, meta.UploadURL, bytes.NewReader(content))
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT upload URL: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT upload URL status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	body, contentType, err := svc.Download(ctx, meta.Key)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if !bytes.Equal(got, content) {
		t.Errorf("Download body = %q, want %q", got, content)
	}
	if contentType != "application/pdf" {
		t.Errorf("Download content type = %q, want application/pdf", contentType)
	}

	if err := svc.Delete(ctx, meta.Key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := svc.Download(ctx, meta.Key); err == nil {
		t.Fatal("Download after Delete: expected error, got nil")
	}
}

func TestProxyHandler_UploadAndDownload(t *testing.T) {
	stack := newProxyStack(t, ownerToken)

	rec := httptest.NewRecorder()
	stack.Handler.Upload(rec, httptest.NewRequest(http.MethodPost, "/api/v1/storage",
		strings.NewReader(`{"filename":"a.pdf","mime_type":"application/pdf","size":10}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Upload status = %d, body %s", rec.Code, rec.Body.String())
	}
	var meta corestorage.FileMetadata
	if err := json.NewDecoder(rec.Body).Decode(&meta); err != nil || meta.Key == "" || meta.UploadURL == "" {
		t.Fatalf("Upload response = %+v (%v), want key and upload_url", meta, err)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/storage/"+meta.Key, nil)
	req.SetPathValue("key", meta.Key)
	stack.Handler.Download(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Download status = %d, body %s", rec.Code, rec.Body.String())
	}
	var dl struct {
		DownloadURL string `json:"download_url"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&dl); err != nil || dl.DownloadURL == "" || dl.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("Download response = %+v (%v), want the owner's download_url and a future expires_at", dl, err)
	}
}

// A request the owning service rejects is relayed with its status and message.
func TestProxyHandler_RelaysOwnerRejection(t *testing.T) {
	stack := newProxyStack(t, ownerToken)

	rec := httptest.NewRecorder()
	stack.Handler.Upload(rec, httptest.NewRequest(http.MethodPost, "/api/v1/storage",
		strings.NewReader(`{"filename":"a.exe","mime_type":"application/x-msdownload","size":10}`)))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("Upload status = %d, want %d; body %s", rec.Code, http.StatusUnsupportedMediaType, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid or prohibited file type") {
		t.Errorf("Upload body = %s, want the owning service's message", rec.Body.String())
	}
}

// The owning service refusing this service's credentials is not the caller's
// fault: it is reported as a gateway failure, never relayed as a 401.
func TestProxyHandler_OwnerRefusingCredentialsIsBadGateway(t *testing.T) {
	stack := newProxyStack(t, "wrong-token")

	rec := httptest.NewRecorder()
	stack.Handler.Upload(rec, httptest.NewRequest(http.MethodPost, "/api/v1/storage",
		strings.NewReader(`{"filename":"a.pdf","mime_type":"application/pdf","size":10}`)))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("Upload status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestProxyHandler_RejectsIncompleteUpload(t *testing.T) {
	h := NewProxyHandler(&ProxyService{})
	for _, body := range []string{`not json`, `{"mime_type":"application/pdf","size":1}`, `{"filename":"a.pdf","size":1}`, `{"filename":"a.pdf","mime_type":"application/pdf"}`} {
		rec := httptest.NewRecorder()
		h.Upload(rec, httptest.NewRequest(http.MethodPost, "/api/v1/storage", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Upload(%s) status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestNewProxyService_UnknownServiceFailsAtStartup(t *testing.T) {
	cfg := defaultProxyConfig()
	cfg.Service = "not-registered"
	if _, err := NewProxyService(newRegistry(t, "https://files.example.com", ownerToken), cfg); err == nil {
		t.Fatal("NewProxyService: expected error for a service missing from the registry")
	}
}

func TestNew_BackendModeKeepsCoreStorage(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := stack.Service.(*corestorage.Service); !ok {
		t.Errorf("Service = %T, want *corestorage.Service", stack.Service)
	}
	if stack.LocalContent == nil {
		t.Error("LocalContent = nil, want the local content handlers for a local backend")
	}
}

func TestNew_ProxyModeHasNoLocalContent(t *testing.T) {
	stack := newProxyStack(t, ownerToken)
	if _, ok := stack.Service.(*ProxyService); !ok {
		t.Errorf("Service = %T, want *ProxyService", stack.Service)
	}
	if stack.LocalContent != nil {
		t.Error("LocalContent != nil, want no local content routes in proxy mode")
	}
}

func TestProxyConfigValidate(t *testing.T) {
	cases := map[string]func(*ProxyConfig){
		"missing service":           func(c *ProxyConfig) { c.Service = "" },
		"missing upload path":       func(c *ProxyConfig) { c.UploadPath = "" },
		"download path without key": func(c *ProxyConfig) { c.DownloadPath = "/api/v1/storage" },
		"delete path without key":   func(c *ProxyConfig) { c.DeletePath = "/api/v1/storage" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := defaultProxyConfig()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("Validate: expected error")
			}
		})
	}
	if err := defaultProxyConfig().Validate(); err != nil {
		t.Errorf("Validate(defaults) error = %v", err)
	}
}

func TestKeyPath_KeyAnywhereInPath(t *testing.T) {
	if got := keyPath("/api/v1/{key}/content", "abc.pdf"); got != "/api/v1/abc.pdf/content" {
		t.Errorf("keyPath = %q, want /api/v1/abc.pdf/content", got)
	}
}
