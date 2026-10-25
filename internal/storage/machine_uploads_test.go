package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
)

const (
	htmlClient  = "NPQS_TO_NSW"
	otherClient = "CDA_TO_NSW"
	generated   = "text/html; charset=utf-8"
)

func client(id string) *authn.Principal {
	return &authn.Principal{Kind: authn.KindClient, ClientID: id}
}

// internalUpload calls h.InternalUpload with body as p (no principal when p is nil).
func internalUpload(h *FileHandler, p *authn.Principal, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/storage", strings.NewReader(body))
	if p != nil {
		req = req.WithContext(authn.ContextWithPrincipal(req.Context(), p))
	}
	rec := httptest.NewRecorder()
	h.InternalUpload(rec, req)
	return rec
}

func uploadBody(mimeType string) string {
	b, _ := json.Marshal(map[string]any{"filename": "doc", "mime_type": mimeType, "size": 10})
	return string(b)
}

// machineStack builds a local stack that lists text/html as a type it opens in
// the browser, as a TNSW deployment does for the documents it generates, with
// the NPQS client granted text/html.
func machineStack(t *testing.T) (*Stack, *FileHandler, *fileaccess.Access) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfg := localConfig(t, srv.URL)
	cfg.InlineTypes = append(corestorage.DefaultInlineTypes(), "text/html")
	stack, err := New(context.Background(), cfg, nil, WithInternalUploadTypes(map[string][]string{htmlClient: {"text/html"}}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stack.LocalContent.RegisterRoutes(mux)
	access, _ := testFileAccess(t)
	return stack, NewFileHandler(stack.Service, access, WithMachineUploads(stack.MachineUploads)), access
}

func TestMachineUploads_TypesByClient(t *testing.T) {
	_, h, _ := machineStack(t)

	tests := []struct {
		name     string
		p        *authn.Principal
		mimeType string
		want     int
	}{
		{"a type users may upload", client(otherClient), "application/pdf", http.StatusOK},
		{"case and parameters are ignored", client(otherClient), "IMAGE/PNG; q=1", http.StatusOK},
		{"HTML for a client not granted it", client(otherClient), "text/html", http.StatusUnsupportedMediaType},
		{"HTML as generated, for a client not granted it", client(otherClient), generated, http.StatusUnsupportedMediaType},
		{"HTML as generated, for the client granted it", client(htmlClient), generated, http.StatusOK},
		{"HTML in capitals, for the client granted it", client(htmlClient), "TEXT/HTML", http.StatusOK},
		{"the base list still applies to the client granted HTML", client(htmlClient), "application/pdf", http.StatusOK},
		{"another document type the client was not granted", client(htmlClient), "image/svg+xml", http.StatusUnsupportedMediaType},
		{"an unlisted type", client(otherClient), "application/zip", http.StatusUnsupportedMediaType},
		{"a value that is not a media type", client(htmlClient), "text/ html", http.StatusUnsupportedMediaType},
		{"a value with a malformed parameter", client(htmlClient), "text/html; charset", http.StatusUnsupportedMediaType},
		{"a caller with no client ID gets the base list only", user("alice"), "text/html", http.StatusUnsupportedMediaType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertStatus(t, internalUpload(h, tt.p, uploadBody(tt.mimeType)), tt.want)
		})
	}
}

// A user keeps the list they always had: the types a machine client is granted
// do not reach the user route.
func TestMachineUploads_DoNotWidenTheUserRoute(t *testing.T) {
	_, h, _ := machineStack(t)
	assertStatus(t, upload(h, client(htmlClient), uploadBody(generated)), http.StatusUnsupportedMediaType)
	assertStatus(t, upload(h, user("alice"), uploadBody("application/pdf")), http.StatusOK)
}

// The internal route issues the caller a reference that opens the file as well
// as submits it, and what it stores is served by the deployment's own policy:
// the type as sent, opened inline only when inlineTypes says so.
func TestMachineUploads_StoreWhatWasSent(t *testing.T) {
	stack, h, access := machineStack(t)
	caller := client(htmlClient)

	rec := internalUpload(h, caller, uploadBody(generated))
	assertStatus(t, rec, http.StatusOK)
	var meta struct {
		Key       string `json:"key"`
		UploadURL string `json:"upload_url"`
		MimeType  string `json:"mime_type"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta.MimeType != generated {
		t.Errorf("mime_type = %q, want the type as sent, %q", meta.MimeType, generated)
	}
	for _, scope := range []fileaccess.Scope{fileaccess.ScopeRead, fileaccess.ScopeSubmit} {
		if _, err := access.Resolve(caller, meta.Key, scope); err != nil {
			t.Errorf("the reference does not give the client %v: %v", scope, err)
		}
	}
	if _, err := access.Resolve(client(otherClient), meta.Key, fileaccess.ScopeRead); err == nil {
		t.Error("another client can use the reference")
	}

	putFile(t, meta.UploadURL, generated, 10)
	stored, err := access.Resolve(caller, meta.Key, fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	dl, err := stack.Service.DownloadURL(context.Background(), stored)
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if dl.ContentType != generated || dl.Disposition != drivers.DispositionInline {
		t.Errorf("stored as %q served %q, want %q served %q", dl.ContentType, dl.Disposition, generated, drivers.DispositionInline)
	}
}

func TestMachineUploads_Rejects(t *testing.T) {
	_, h, _ := machineStack(t)
	pdf := `{"filename":"a.pdf","mime_type":"application/pdf","size":10}`
	assertStatus(t, internalUpload(h, nil, pdf), http.StatusUnauthorized)
	assertStatus(t, internalUpload(h, client(otherClient), `{`), http.StatusBadRequest)
	assertStatus(t, internalUpload(h, client(otherClient), `{"filename":"a.pdf","size":10}`), http.StatusBadRequest)
	assertStatus(t, internalUpload(h, client(otherClient), `{"filename":"a.pdf","mime_type":"application/pdf"}`), http.StatusBadRequest)
	tooLarge := `{"filename":"a.pdf","mime_type":"application/pdf","size":` + "2000000" + `}`
	assertStatus(t, internalUpload(h, client(otherClient), tooLarge), http.StatusBadRequest)
}

func TestNew_MachineUploadsOnlyWithABackend(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if stack.MachineUploads == nil {
		t.Error("a local backend has no machine uploads")
	}
	if h := NewFileHandler(stack.Service, failingRefs{}); h.HasMachineUploads() {
		t.Error("a handler built without WithMachineUploads reports machine uploads")
	}

	proxy := newProxyStack(t, ownerToken)
	if proxy.MachineUploads != nil {
		t.Error("a proxy has machine uploads: nothing uploads to a deployment that stores nothing")
	}
}

// uploadCounter is a Service that records what reaches it.
type uploadCounter struct {
	Service
	uploads int
}

func (u *uploadCounter) Upload(context.Context, string, int64, string) (*corestorage.FileMetadata, error) {
	u.uploads++
	return &corestorage.FileMetadata{Key: "k", UploadURL: "http://x/put"}, nil
}

// WithUploadTypes keeps a user to their list before anything is sent to the
// owning service, which accepts more from this deployment than its users may
// upload.
func TestFileHandler_UploadTypesLimitUsers(t *testing.T) {
	access, _ := testFileAccess(t)
	svc := &uploadCounter{}
	h := NewFileHandler(svc, access, WithUploadTypes([]string{"application/pdf"}))

	for _, mimeType := range []string{"text/html", generated, "image/svg+xml", "application/zip", "text/ html", "application/pdf2"} {
		assertStatus(t, upload(h, user("alice"), uploadBody(mimeType)), http.StatusUnsupportedMediaType)
	}
	if svc.uploads != 0 {
		t.Errorf("%d refused uploads reached the owning service", svc.uploads)
	}
	assertStatus(t, upload(h, user("alice"), uploadBody("Application/PDF; x=y")), http.StatusOK)
	if svc.uploads != 1 {
		t.Errorf("%d uploads reached the owning service, want 1", svc.uploads)
	}
}

func TestConfigValidate_InternalUploadTypes(t *testing.T) {
	tests := []struct {
		name    string
		types   map[string][]string
		wantErr string
	}{
		{name: "none"},
		{name: "a document type a client generates", types: map[string][]string{"npqs": {"text/html"}}},
		{name: "a client with no extras", types: map[string][]string{"npqs": nil}},
		{name: "a parameter", types: map[string][]string{"npqs": {"text/html; charset=utf-8"}}, wantErr: "storage.allowedInternalUploadTypes.npqs"},
		{name: "a wildcard", types: map[string][]string{"npqs": {"text/*"}}, wantErr: "storage.allowedInternalUploadTypes.npqs"},
		{name: "no client name", types: map[string][]string{" ": {"text/html"}}, wantErr: "a client name is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := localConfig(t, "http://localhost:8080")
			cfg.InlineTypes = append(corestorage.DefaultInlineTypes(), "text/html")
			cfg.AllowedInternalUploadTypes = tt.types
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigValidate_ProxyUploadTypes(t *testing.T) {
	tests := []struct {
		name    string
		types   []string
		wantErr string
	}{
		{name: "a list", types: []string{"application/pdf", "image/png"}},
		{name: "none", wantErr: "storage.proxy.allowedUploadTypes must list at least one"},
		{name: "a parameter", types: []string{"application/pdf; q=1"}, wantErr: "storage.proxy.allowedUploadTypes"},
		{name: "HTML", types: []string{"application/pdf", "text/html"}, wantErr: "text/html runs script"},
		{name: "XHTML", types: []string{"application/xhtml+xml"}, wantErr: "application/xhtml+xml runs script"},
		{name: "SVG", types: []string{"image/svg+xml"}, wantErr: "image/svg+xml runs script"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Config: corestorage.Config{Type: TypeProxy}, Proxy: defaultProxyConfig(), TokenKeyset: testTokenKeyset}
			cfg.Proxy.AllowedUploadTypes = tt.types
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}
