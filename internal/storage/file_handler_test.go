package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
)

// testFileAccess returns real file tokens over testTokenKeyset, on a clock the
// test can move.
func testFileAccess(t *testing.T) (*fileaccess.Access, *time.Time) {
	t.Helper()
	now := time.Now()
	keys, err := filetoken.ParseKeyset(testTokenKeyset)
	if err != nil {
		t.Fatal(err)
	}
	codec, err := filetoken.NewCodec(keys, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	access, err := fileaccess.New(codec, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return access, &now
}

func user(sub string) *authn.Principal {
	return &authn.Principal{Kind: authn.KindUser, IDPUserID: sub}
}

// download calls h.Download for ref as p (no principal when p is nil).
func download(h *FileHandler, ref string, p *authn.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/storage/"+url.PathEscape(ref), nil)
	req.SetPathValue("key", ref)
	if p != nil {
		req = req.WithContext(authn.ContextWithPrincipal(req.Context(), p))
	}
	rec := httptest.NewRecorder()
	h.Download(rec, req)
	return rec
}

func assertDownloadURL(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		DownloadURL string `json:"download_url"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.DownloadURL == "" || body.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("response = %+v (%v), want a download_url and a future expires_at", body, err)
	}
}

// assertFileType checks that a successful download gives the file's
// extension, the type it was stored with and how the URL serves it, and no
// name for it.
func assertFileType(t *testing.T, rec *httptest.ResponseRecorder, ext, mimeType, disposition string) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["extension"] != ext || body["mime_type"] != mimeType || body["disposition"] != disposition {
		t.Errorf("extension, mime_type, disposition = %v, %v, %v; want %q, %q, %q", body["extension"], body["mime_type"], body["disposition"], ext, mimeType, disposition)
	}
	if _, ok := body["name"]; ok {
		t.Errorf("response %s names the file", rec.Body.String())
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, want, rec.Body.String())
	}
}

// storedKey stores a PDF with svc and returns its key. A download is only
// issued for a file the backend holds.
func storedKey(t *testing.T, svc Service) string {
	t.Helper()
	meta, err := svc.Save(context.Background(), "a.pdf", "application/pdf", strings.NewReader("%PDF-1.4"), 8)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return meta.Key
}

// reservedKey reserves a key with svc, as an upload would, without storing a
// file under it.
func reservedKey(t *testing.T, svc Service) string {
	t.Helper()
	meta, err := svc.Upload(context.Background(), "a.pdf", 10, "application/pdf")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	return meta.Key
}

func TestFileHandler_Download(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	access, now := testFileAccess(t)
	h := handlerOf(stack.Service, access)
	key := storedKey(t, stack.Service)
	token, err := access.Issue(user("alice"), key, fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the holder's token", func(t *testing.T) {
		rec := download(h, token, user("alice"))
		assertDownloadURL(t, rec)
		assertFileType(t, rec, ".pdf", "application/pdf", drivers.DispositionInline)
	})
	t.Run("a file the backend does not hold", func(t *testing.T) {
		missing, err := access.Issue(user("alice"), reservedKey(t, stack.Service), fileaccess.ScopeRead)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, download(h, missing, user("alice")), http.StatusNotFound)
	})
	t.Run("someone else's token", func(t *testing.T) {
		assertStatus(t, download(h, token, user("bob")), http.StatusForbidden)
	})
	t.Run("a token that can't be opened", func(t *testing.T) {
		submitOnly, err := access.Issue(user("alice"), key, fileaccess.ScopeSubmit)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, download(h, submitOnly, user("alice")), http.StatusForbidden)
	})
	// A raw storage key grants nothing, even to whoever uploaded the file.
	t.Run("a stored value", func(t *testing.T) {
		assertStatus(t, download(h, key, user("alice")), http.StatusForbidden)
	})
	t.Run("anything else that is not a reference", func(t *testing.T) {
		assertStatus(t, download(h, "../outside", user("alice")), http.StatusForbidden)
	})
	t.Run("no principal", func(t *testing.T) {
		assertStatus(t, download(h, token, nil), http.StatusUnauthorized)
	})
	t.Run("an expired token", func(t *testing.T) {
		*now = now.Add(time.Hour)
		assertStatus(t, download(h, token, user("alice")), http.StatusGone)
	})
}

type failingRefs struct{}

func (failingRefs) Issue(*authn.Principal, string, ...fileaccess.Scope) (string, error) {
	return "", errors.New("keyset unavailable")
}

func (failingRefs) Resolve(*authn.Principal, string, fileaccess.Scope) (string, error) {
	return "", errors.New("keyset unavailable")
}

func TestFileHandler_DownloadResolveFailure(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	assertStatus(t, download(handlerOf(stack.Service, failingRefs{}), "ref", user("alice")), http.StatusInternalServerError)
}

// newFileAccess returns file tokens over its own one-key keyset filled with b.
func newFileAccess(t *testing.T, b byte) *fileaccess.Access {
	t.Helper()
	keys, err := filetoken.ParseKeyset("k:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	codec, err := filetoken.NewCodec(keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	access, err := fileaccess.New(codec, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return access
}

// upload calls h.Upload with body as p (no principal when p is nil).
func upload(h *FileHandler, p *authn.Principal, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/storage", strings.NewReader(body))
	if p != nil {
		req = req.WithContext(authn.ContextWithPrincipal(req.Context(), p))
	}
	rec := httptest.NewRecorder()
	h.Upload(rec, req)
	return rec
}

// putFile uploads size bytes to uploadURL as contentType, as a client does with
// the URL an upload returned.
func putFile(t *testing.T, uploadURL, contentType string, size int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(make([]byte, size)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT upload URL: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT upload URL status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestFileHandler_UploadReturnsAReference(t *testing.T) {
	// The content routes serve the upload URLs it returns.
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	stack, err := New(context.Background(), localConfig(t, srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stack.LocalContent.RegisterRoutes(mux)
	access, _ := testFileAccess(t)
	h := handlerOf(stack.Service, access)

	rec := upload(h, user("alice"), `{"filename":"invoice.pdf","mime_type":"application/pdf","size":10}`)
	assertStatus(t, rec, http.StatusOK)
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["id"]; ok {
		t.Errorf("response %v has an id, which is the stored key without its extension", body)
	}
	if body["name"] != "invoice.pdf" || body["upload_url"] == "" {
		t.Errorf("response %v, want the name and an upload_url", body)
	}
	ref, _ := body["key"].(string)
	stored, err := access.Resolve(user("alice"), ref, fileaccess.ScopeSubmit)
	if err != nil {
		t.Fatalf("key %q is not a reference for the uploader: %v", ref, err)
	}
	if !strings.Contains(body["upload_url"].(string), stored) {
		t.Errorf("upload_url %v does not name the stored file %q", body["upload_url"], stored)
	}
	// It can only be submitted: opening the file takes the reference a task
	// gives its reader.
	rec = download(h, ref, user("alice"))
	assertStatus(t, rec, http.StatusForbidden)
	if !strings.Contains(rec.Body.String(), "save the form to view it") {
		t.Errorf("body %s, want it to say to save the form to view the file", rec.Body.String())
	}
	assertStatus(t, download(h, ref, user("bob")), http.StatusForbidden)
	read, err := access.Issue(user("alice"), stored, fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	// Until the file is uploaded there is nothing to open.
	assertStatus(t, download(h, read, user("alice")), http.StatusNotFound)
	putFile(t, body["upload_url"].(string), "application/pdf", 10)
	assertDownloadURL(t, download(h, read, user("alice")))
}

func TestFileHandler_UploadRejects(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	access, _ := testFileAccess(t)
	h := handlerOf(stack.Service, access)

	tests := []struct {
		name, body  string
		p           *authn.Principal
		wantStatus  int
		wantMessage string
	}{
		{"no principal", `{"filename":"a.pdf","mime_type":"application/pdf","size":10}`, nil, http.StatusUnauthorized, "authentication required"},
		{"bad body", `{`, user("alice"), http.StatusBadRequest, "invalid request body"},
		{"no filename", `{"mime_type":"application/pdf","size":10}`, user("alice"), http.StatusBadRequest, "filename is required"},
		{"no type", `{"filename":"a.pdf","size":10}`, user("alice"), http.StatusBadRequest, "mime_type is required"},
		{"no size", `{"filename":"a.pdf","mime_type":"application/pdf"}`, user("alice"), http.StatusBadRequest, "size must be greater than 0"},
		{"too large", fmt.Sprintf(`{"filename":"a.pdf","mime_type":"application/pdf","size":%d}`, testMaxUploadBytes+1), user("alice"), http.StatusBadRequest, "file size exceeds 1MB limit"},
		{"type not allowed", `{"filename":"a.exe","mime_type":"application/x-msdownload","size":10}`, user("alice"), http.StatusUnsupportedMediaType, "invalid or prohibited file type: application/x-msdownload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := upload(h, tt.p, tt.body)
			assertStatus(t, rec, tt.wantStatus)
			if !strings.Contains(rec.Body.String(), tt.wantMessage) {
				t.Errorf("body %s, want it to say %q", rec.Body.String(), tt.wantMessage)
			}
		})
	}

	t.Run("no reference can be issued", func(t *testing.T) {
		rec := upload(handlerOf(stack.Service, failingRefs{}), user("alice"), `{"filename":"a.pdf","mime_type":"application/pdf","size":10}`)
		assertStatus(t, rec, http.StatusInternalServerError)
	})
}

// A backend of this deployment's own failing is this service's error (500),
// not a gateway's.
func TestFileHandler_DownloadBackendFailureIsInternal(t *testing.T) {
	access, _ := testFileAccess(t)
	token, err := access.Issue(user("alice"), "any-key.pdf", fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, download(handlerOf(failingService{}, access), token, user("alice")), http.StatusInternalServerError)
}

// failingService is a Service whose backend fails every call.
type failingService struct{ Service }

func (failingService) DownloadURL(context.Context, string) (corestorage.Download, error) {
	return corestorage.Download{}, errors.New("backend unavailable")
}

// A download says what storage says: the type the file was stored with and how
// its URL is served. The key's extension only names a file the client saves,
// so a filename that disagrees with the type changes neither.
func TestDownloadOf_ReportsWhatStorageSays(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, tc := range []struct {
		name, filename, mimeType string
		wantExt, wantDisposition string
	}{
		{name: "a PDF", filename: "a.pdf", mimeType: "application/pdf", wantExt: ".pdf", wantDisposition: drivers.DispositionInline},
		{name: "XML", filename: "a.xml", mimeType: "text/xml", wantExt: ".xml", wantDisposition: drivers.DispositionAttachment},
		{name: "generated HTML", filename: "a.HTML", mimeType: "text/html; charset=utf-8", wantExt: ".HTML", wantDisposition: drivers.DispositionAttachment},
		{name: "XML named like a PDF", filename: "draft.pdf", mimeType: "text/xml", wantExt: ".pdf", wantDisposition: drivers.DispositionAttachment},
		{name: "a PDF named like XML", filename: "report.xml", mimeType: "application/pdf", wantExt: ".xml", wantDisposition: drivers.DispositionInline},
		{name: "an extension nothing maps to a type", filename: "a.unknownext", mimeType: "image/png", wantExt: ".unknownext", wantDisposition: drivers.DispositionInline},
		{name: "no extension", filename: "a", mimeType: "image/png", wantExt: "", wantDisposition: drivers.DispositionInline},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := stack.Service.Save(context.Background(), tc.filename, tc.mimeType, strings.NewReader("data"), 4)
			if err != nil {
				t.Fatalf("Save: %v", err)
			}
			dl, err := downloadOf(context.Background(), stack.Service, meta.Key)
			if err != nil {
				t.Fatalf("downloadOf: %v", err)
			}
			if dl.Extension != tc.wantExt || dl.MimeType != tc.mimeType || dl.Disposition != tc.wantDisposition {
				t.Errorf("extension, mime_type, disposition = %q, %q, %q; want %q, %q, %q",
					dl.Extension, dl.MimeType, dl.Disposition, tc.wantExt, tc.mimeType, tc.wantDisposition)
			}
		})
	}
}

// handlerOf returns a FileHandler over svc, a deployment that has no peers: every
// file in it is its own.
func handlerOf(svc Service, refs FileRefs) *FileHandler {
	return NewFileHandler(NewFiles(svc, nil, nil, nil), refs)
}

// Only users upload. A machine client stores its files in its own storage, so a
// reservation by one is refused before anything is reserved.
func TestFileHandler_UploadRefusesMachineClients(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	access, _ := testFileAccess(t)
	h := handlerOf(stack.Service, access)
	client := &authn.Principal{Kind: authn.KindClient, ClientID: "CDA_TO_NSW"}

	rec := upload(h, client, `{"filename":"a.pdf","mime_type":"application/pdf","size":10}`)

	assertStatus(t, rec, http.StatusForbidden)
	if strings.Contains(rec.Body.String(), "upload_url") {
		t.Errorf("response %s reserved an upload", rec.Body.String())
	}
}
