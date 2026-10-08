package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	mux.HandleFunc(DownloadRoute, func(w http.ResponseWriter, r *http.Request) {
		client := &authn.Principal{Kind: authn.KindClient, ClientID: agencyClient}
		tnswFiles.Download(w, r.WithContext(authn.ContextWithPrincipal(r.Context(), client)))
	})
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
}
