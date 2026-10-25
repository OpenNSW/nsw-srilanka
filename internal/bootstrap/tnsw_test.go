package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
)

// uploaderStub prepares every upload the same way.
type uploaderStub struct{}

func (uploaderStub) Upload(context.Context, string, int64, string) (*corestorage.FileMetadata, error) {
	return &corestorage.FileMetadata{Key: "k", UploadURL: "http://storage.test/put", Name: "doc", MimeType: "application/pdf", Size: 10}, nil
}

type refsStub struct{}

func (refsStub) Issue(*authn.Principal, string, ...fileaccess.Scope) (string, error) {
	return "ref", nil
}
func (refsStub) Resolve(*authn.Principal, string, fileaccess.Scope) (string, error) { return "", nil }

// The internal upload route is open to a caller holding the machine upload
// scope, not to one holding only the scope users upload with, and is not served
// when the deployment has no uploads of its own to prepare.
func TestMountTNSW_InternalUpload(t *testing.T) {
	// Callers present the scopes they hold in a header; the scope check reads it.
	withAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := &authn.Principal{Kind: authn.KindClient, ClientID: "CDA_TO_NSW"}
			next.ServeHTTP(w, r.WithContext(authn.ContextWithPrincipal(r.Context(), p)))
		})
	}
	withScope := func(scope string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !slices.Contains(strings.Fields(r.Header.Get("X-Scopes")), scope) {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}
	post := func(mux *http.ServeMux, scope string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/storage", strings.NewReader(`{"filename":"a.pdf","mime_type":"application/pdf","size":10}`))
		req.Header.Set("X-Scopes", scope)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	mux := http.NewServeMux()
	mountTNSW(mux, tnswHandlers{files: nswstorage.NewFileHandler(nil, refsStub{}, nswstorage.WithMachineUploads(uploaderStub{}))}, withAuth, withScope)
	if got := post(mux, scopes.StorageInternalWrite); got != http.StatusOK {
		t.Errorf("with %s: status %d, want %d", scopes.StorageInternalWrite, got, http.StatusOK)
	}
	if got := post(mux, scopes.StorageWrite); got != http.StatusForbidden {
		t.Errorf("with only %s: status %d, want %d", scopes.StorageWrite, got, http.StatusForbidden)
	}
	if got := post(mux, ""); got != http.StatusForbidden {
		t.Errorf("with no scope: status %d, want %d", got, http.StatusForbidden)
	}

	for name, h := range map[string]tnswHandlers{
		"no file handler":                {},
		"a file handler with no uploads": {files: nswstorage.NewFileHandler(nil, refsStub{})},
	} {
		mux := http.NewServeMux()
		mountTNSW(mux, h, withAuth, withScope)
		if got := post(mux, scopes.StorageInternalWrite); got != http.StatusNotFound {
			t.Errorf("%s: status %d, want %d (not served)", name, got, http.StatusNotFound)
		}
	}
}

func TestInternalUploadTypesByClient(t *testing.T) {
	cat := &catalog.Catalog{Clients: map[string]string{"npqs": "NPQS_TO_NSW", "cda": "CDA_TO_NSW"}}

	got, err := internalUploadTypesByClient(nswstorage.Config{AllowedInternalUploadTypes: map[string][]string{"npqs": {"text/html"}}}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"text/html"}; !slices.Equal(got["NPQS_TO_NSW"], want) || len(got) != 1 {
		t.Errorf("types by client = %v, want NPQS_TO_NSW: %v", got, want)
	}

	if got, err := internalUploadTypesByClient(nswstorage.Config{}, cat); err != nil || len(got) != 0 {
		t.Errorf("with none configured: %v, %v, want an empty map", got, err)
	}

	_, err = internalUploadTypesByClient(nswstorage.Config{AllowedInternalUploadTypes: map[string][]string{"nobody": {"text/html"}}}, cat)
	if err == nil || !strings.Contains(err.Error(), `"nobody" is not a client in the catalog`) {
		t.Errorf("an unknown client: %v, want an error naming it", err)
	}
}
