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

	// Without uploads of its own, a deployment does not serve the route.
	mux = http.NewServeMux()
	mountTNSW(mux, tnswHandlers{files: nswstorage.NewFileHandler(nil, refsStub{})}, withAuth, withScope)
	if got := post(mux, scopes.StorageInternalWrite); got != http.StatusNotFound {
		t.Errorf("with no uploads: status %d, want %d (not served)", got, http.StatusNotFound)
	}
}
