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
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.DownloadURL == "" || body.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("response = %+v (%v), want a download_url and a future expires_at", body, err)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, want, rec.Body.String())
	}
}

// uploadKey reserves a key with svc, as an upload would.
func uploadKey(t *testing.T, svc Service) string {
	t.Helper()
	meta, err := svc.Upload(context.Background(), "a.pdf", 10, "application/pdf")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	return meta.Key
}

func TestFileHandler_Download(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	access, now := testFileAccess(t)
	h := NewFileHandler(stack.Service, access)
	key := uploadKey(t, stack.Service)
	token, err := access.IssueFor(user("alice"), key)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the holder's token", func(t *testing.T) {
		assertDownloadURL(t, download(h, token, user("alice")))
	})
	t.Run("someone else's token", func(t *testing.T) {
		assertStatus(t, download(h, token, user("bob")), http.StatusForbidden)
	})
	// Until every caller holds file tokens.
	t.Run("a stored value", func(t *testing.T) {
		assertDownloadURL(t, download(h, key, user("bob")))
	})
	t.Run("a value core/storage could not have made", func(t *testing.T) {
		assertStatus(t, download(h, "../outside", user("alice")), http.StatusBadRequest)
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

func (failingRefs) IssueFor(*authn.Principal, string) (string, error) {
	return "", errors.New("keyset unavailable")
}

func (failingRefs) Resolve(*authn.Principal, string) (string, error) {
	return "", errors.New("keyset unavailable")
}

func TestFileHandler_DownloadResolveFailure(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	assertStatus(t, download(NewFileHandler(stack.Service, failingRefs{}), "ref", user("alice")), http.StatusInternalServerError)
}

// In proxy mode the stored value is the owning service's own reference, and
// its answers are relayed as the proxy's own handler relays them.
func TestFileHandler_DownloadThroughProxy(t *testing.T) {
	access, _ := testFileAccess(t)

	t.Run("the holder's token", func(t *testing.T) {
		stack := newProxyStack(t, ownerToken)
		h := NewFileHandler(stack.Service, access)
		token, err := access.IssueFor(user("alice"), uploadKey(t, stack.Service))
		if err != nil {
			t.Fatal(err)
		}
		assertDownloadURL(t, download(h, token, user("alice")))
	})
	t.Run("the owner's rejection is relayed", func(t *testing.T) {
		h := NewFileHandler(newProxyStack(t, ownerToken).Service, access)
		token, err := access.IssueFor(user("alice"), "not-a-key")
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, download(h, token, user("alice")), http.StatusBadRequest)
	})
	t.Run("the owner refusing credentials is a gateway failure", func(t *testing.T) {
		h := NewFileHandler(newProxyStack(t, "wrong-token").Service, access)
		token, err := access.IssueFor(user("alice"), "any-key")
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, download(h, token, user("alice")), http.StatusBadGateway)
	})
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

// An agency runs the same handler in proxy mode. What it stores is the token
// TNSW gave it, which it hands its officer wrapped in a token of its own; the
// officer's download resolves both, one at each end.
func TestFileHandler_AgencyOfficerDownloadsThroughTNSW(t *testing.T) {
	const agencyClient = "CDA_TO_NSW"
	tnswAccess, agencyAccess := newFileAccess(t, 1), newFileAccess(t, 2)

	// TNSW: its own backend, with the agency calling as its machine client.
	mux := http.NewServeMux()
	tnsw := httptest.NewServer(mux)
	t.Cleanup(tnsw.Close)
	driver, err := drivers.NewLocalFSDriver(t.TempDir(), tnsw.URL, "owner-secret", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tnswService := corestorage.NewService(driver, corestorage.WithAllowedUploadTypes(testUploadTypes...))
	tnswFiles := NewFileHandler(tnswService, tnswAccess)
	asAgency := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			client := &authn.Principal{Kind: authn.KindClient, ClientID: agencyClient}
			next(w, r.WithContext(authn.ContextWithPrincipal(r.Context(), client)))
		}
	}
	mux.HandleFunc(UploadRoute, asAgency(tnswFiles.Upload))
	mux.HandleFunc(DownloadRoute, asAgency(tnswFiles.Download))
	corestorage.NewLocalContentHandler(driver).RegisterRoutes(mux)

	key := uploadKey(t, tnswService)
	pushed, err := tnswAccess.IssueForClient(agencyClient, key)
	if err != nil {
		t.Fatal(err)
	}

	// The agency: a proxy onto TNSW.
	agencyService, err := NewProxyService(newRegistry(t, tnsw.URL, ownerToken), defaultProxyConfig())
	if err != nil {
		t.Fatal(err)
	}
	agencyFiles := NewFileHandler(agencyService, agencyAccess)
	officer := user("cda-officer")
	officerToken, err := agencyAccess.IssueFor(officer, pushed)
	if err != nil {
		t.Fatal(err)
	}

	assertDownloadURL(t, download(agencyFiles, officerToken, officer))
	assertStatus(t, download(agencyFiles, officerToken, user("another-officer")), http.StatusForbidden)

	// An officer's own upload goes to TNSW too: TNSW issues the agency a
	// token, and the agency wraps it for the officer.
	uploaded := upload(agencyFiles, officer, `{"filename":"cert.pdf","mime_type":"application/pdf","size":10}`)
	assertStatus(t, uploaded, http.StatusOK)
	var meta struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(uploaded.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	stored, err := agencyAccess.Resolve(officer, meta.Key)
	if err != nil {
		t.Fatalf("the officer's upload reference: %v", err)
	}
	if _, err := tnswAccess.Resolve(&authn.Principal{Kind: authn.KindClient, ClientID: agencyClient}, stored); err != nil {
		t.Fatalf("what the agency stores is not TNSW's token for it: %v", err)
	}
	assertDownloadURL(t, download(agencyFiles, meta.Key, officer))
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

func TestFileHandler_UploadReturnsAReference(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	access, _ := testFileAccess(t)
	h := NewFileHandler(stack.Service, access)

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
	stored, err := access.Resolve(user("alice"), ref)
	if err != nil {
		t.Fatalf("key %q is not a reference for the uploader: %v", ref, err)
	}
	if !strings.Contains(body["upload_url"].(string), stored) {
		t.Errorf("upload_url %v does not name the stored file %q", body["upload_url"], stored)
	}
	assertDownloadURL(t, download(h, ref, user("alice")))
	assertStatus(t, download(h, ref, user("bob")), http.StatusForbidden)
}

func TestFileHandler_UploadRejects(t *testing.T) {
	stack, err := New(context.Background(), localConfig(t, "http://localhost:8080"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	access, _ := testFileAccess(t)
	h := NewFileHandler(stack.Service, access)

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
		{"type not allowed", `{"filename":"a.exe","mime_type":"application/x-msdownload","size":10}`, user("alice"), http.StatusUnsupportedMediaType, "invalid or prohibited file type"},
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
		rec := upload(NewFileHandler(stack.Service, failingRefs{}), user("alice"), `{"filename":"a.pdf","mime_type":"application/pdf","size":10}`)
		assertStatus(t, rec, http.StatusInternalServerError)
	})
}

// In proxy mode the owning service's rejection is relayed, and its failure to
// accept this service's credentials is a gateway failure.
func TestFileHandler_UploadThroughProxy(t *testing.T) {
	access, _ := testFileAccess(t)

	h := NewFileHandler(newProxyStack(t, ownerToken).Service, access)
	assertStatus(t, upload(h, user("alice"), `{"filename":"a.exe","mime_type":"application/x-msdownload","size":10}`), http.StatusUnsupportedMediaType)

	h = NewFileHandler(newProxyStack(t, "wrong-token").Service, access)
	assertStatus(t, upload(h, user("alice"), `{"filename":"a.pdf","mime_type":"application/pdf","size":10}`), http.StatusBadGateway)
}
