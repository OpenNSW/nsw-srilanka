package storage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenNSW/core/remote"
	remoteauth "github.com/OpenNSW/core/remote/auth"
	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileref"
)

const peerBearer = "peer-bearer"

// token has the shape of a sealed file token, a compact JWE with ".." in it.
const token = "eyJhbGciOiJkaXIiLCJlbmMiOiJBMjU2R0NNIn0..AAAAAAAAAAAAAAAA.BBBB-_cc.DDDD"

// peer stands in for the service that owns a file: its storage route answers
// with whatever answer writes, and it records what it was asked.
type peer struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
	auths []string
}

func newPeer(t *testing.T, answer http.HandlerFunc) *peer {
	t.Helper()
	p := &peer{}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.paths = append(p.paths, r.URL.EscapedPath())
		p.auths = append(p.auths, r.Header.Get("Authorization"))
		p.mu.Unlock()
		answer(w, r)
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *peer) hits() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.paths)
}

// answerWith makes a peer answer every request with status and body.
func answerWith(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func peerRegistry(t *testing.T, id, url string) *remote.Manager {
	t.Helper()
	m := remote.NewManager()
	if err := m.SetServices(remote.ServiceConfig{
		ID: id, URL: url,
		Auth: &remoteauth.Config{Type: remoteauth.TypeBearer, Bearer: &remoteauth.BearerConfig{Token: peerBearer}},
	}); err != nil {
		t.Fatalf("SetServices: %v", err)
	}
	return m
}

// ownService is a deployment's own storage, answering for the one key it holds.
type ownService struct{ Service }

func (ownService) DownloadURL(context.Context, string) (corestorage.Download, error) {
	return corestorage.Download{URL: "https://own.example/k", ExpiresAt: time.Now().Add(time.Hour).Unix(), ContentType: "application/pdf", Disposition: "inline"}, nil
}

func (ownService) Download(context.Context, string) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader("own bytes")), "application/pdf", nil
}

// newPeerFiles returns the Files of a deployment whose only peer is "cda", served
// by p, and that opens only PDFs in the browser.
func newPeerFiles(t *testing.T, p *peer) *Files {
	t.Helper()
	return NewFiles(ownService{}, peerRegistry(t, "cda", p.URL), map[string]string{"cda": p.URL}, []string{"application/pdf"})
}

func foreign(t *testing.T) string {
	t.Helper()
	ref, err := fileref.Foreign("cda", token)
	if err != nil {
		t.Fatal(err)
	}
	return ref.String()
}

func goodAnswer(downloadURL string) string {
	return `{"download_url":"` + downloadURL + `","expires_at":4102444800,"extension":".pdf","mime_type":"application/pdf","disposition":"inline"}`
}

func TestFiles_Locate_OwnValueNeverReachesAPeer(t *testing.T) {
	p := newPeer(t, answerWith(http.StatusOK, goodAnswer("https://cdn.example/x")))
	f := newPeerFiles(t, p)

	dl, err := f.locate(context.Background(), "3f2b9c1e-0a4d-4b7e-9c11-5d6e7f8a9b0c.pdf")
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if dl.DownloadURL != "https://own.example/k" || dl.Extension != ".pdf" || dl.MimeType != "application/pdf" {
		t.Errorf("download = %+v", dl)
	}
	if p.hits() != 0 {
		t.Errorf("the peer was asked %d times about an own file", p.hits())
	}
}

func TestFiles_Locate_ForeignValue(t *testing.T) {
	p := newPeer(t, answerWith(http.StatusOK, goodAnswer("https://cdn.example/x?sig=1")))
	f := newPeerFiles(t, p)

	dl, err := f.locate(context.Background(), foreign(t))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	want := fileDownload{DownloadURL: "https://cdn.example/x?sig=1", ExpiresAt: 4102444800, Extension: ".pdf", MimeType: "application/pdf", Disposition: "inline"}
	if dl != want {
		t.Errorf("download = %+v, want %+v", dl, want)
	}
	if got := p.paths[0]; got != "/api/v1/storage/"+token {
		t.Errorf("path = %q, want the token as one segment", got)
	}
	if got := p.auths[0]; got != "Bearer "+peerBearer {
		t.Errorf("Authorization = %q, want this deployment's credentials", got)
	}
}

// How a file opens is this deployment's decision, from its own inline types.
func TestFiles_Locate_DispositionIsDerived(t *testing.T) {
	tests := []struct {
		name, answer, disposition string
	}{
		{"a type this deployment opens inline", `{"download_url":"https://c.example/x","mime_type":"application/pdf","disposition":"attachment"}`, "inline"},
		{"a type it does not", `{"download_url":"https://c.example/x","mime_type":"text/html","disposition":"inline"}`, "attachment"},
		{"no type", `{"download_url":"https://c.example/x","disposition":"inline"}`, "attachment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPeerFiles(t, newPeer(t, answerWith(http.StatusOK, tt.answer)))
			dl, err := f.locate(context.Background(), foreign(t))
			if err != nil {
				t.Fatalf("locate: %v", err)
			}
			if dl.Disposition != tt.disposition {
				t.Errorf("disposition = %q, want %q", dl.Disposition, tt.disposition)
			}
		})
	}
}

func TestFiles_Locate_DropsMalformedHints(t *testing.T) {
	f := newPeerFiles(t, newPeer(t, answerWith(http.StatusOK,
		`{"download_url":"https://c.example/x","extension":".pdf/../x","mime_type":"application/pdf; charset=evil"}`)))
	dl, err := f.locate(context.Background(), foreign(t))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if dl.Extension != "" || dl.MimeType != "" {
		t.Errorf("extension, type = %q, %q; want both dropped", dl.Extension, dl.MimeType)
	}
}

func TestFiles_Locate_OwnerNotFound(t *testing.T) {
	f := newPeerFiles(t, newPeer(t, answerWith(http.StatusNotFound, `{"error":"secret owner text"}`)))
	_, err := f.locate(context.Background(), foreign(t))
	if !errors.Is(err, corestorage.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if strings.Contains(err.Error(), "secret owner text") {
		t.Errorf("err %q carries the owner's text", err)
	}
}

func TestFiles_Locate_OwnerFailuresAreGatewayFailures(t *testing.T) {
	long := strings.Repeat("a", maxPresignedURLLength)
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"owner refuses our credentials", http.StatusUnauthorized, `{"error":"x"}`},
		{"owner forbids", http.StatusForbidden, `{"error":"x"}`},
		{"owner fails", http.StatusInternalServerError, `oops`},
		{"owner rejects the request", http.StatusBadRequest, `{"error":"secret owner text"}`},
		{"owner redirects", http.StatusFound, ``},
		{"not JSON", http.StatusOK, `<html>`},
		{"no URL", http.StatusOK, `{"expires_at":1}`},
		{"relative URL", http.StatusOK, `{"download_url":"/api/v1/storage/k/content"}`},
		{"protocol-relative URL", http.StatusOK, `{"download_url":"//host/x"}`},
		{"javascript URL", http.StatusOK, `{"download_url":"javascript://host/%0aalert(document.domain)"}`},
		{"data URL", http.StatusOK, `{"download_url":"data:text/html,x"}`},
		{"ftp URL", http.StatusOK, `{"download_url":"ftp://host/x"}`},
		{"URL with credentials", http.StatusOK, `{"download_url":"https://user:pw@host/x"}`},
		{"URL without a host", http.StatusOK, `{"download_url":"https:///x"}`},
		{"oversized URL", http.StatusOK, `{"download_url":"https://host/` + long + `"}`},
		{"oversized answer", http.StatusOK, `{"download_url":"https://host/x","pad":"` + strings.Repeat("a", maxOwnerResponseBytes) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPeerFiles(t, newPeer(t, answerWith(tt.status, tt.body)))
			_, err := f.locate(context.Background(), foreign(t))
			if !isUpstreamFailure(err) {
				t.Fatalf("err = %v, want an upstream failure", err)
			}
			if strings.Contains(err.Error(), "secret owner text") || strings.Contains(err.Error(), token) {
				t.Errorf("err %q carries the owner's text or the token", err)
			}
		})
	}
}

func TestFiles_Locate_OwnerUnreachable(t *testing.T) {
	p := newPeer(t, answerWith(http.StatusOK, goodAnswer("https://c.example/x")))
	f := newPeerFiles(t, p)
	p.Close()
	_, err := f.locate(context.Background(), foreign(t))
	if !isUpstreamFailure(err) {
		t.Fatalf("err = %v, want an upstream failure", err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("err %q names the URL", err)
	}
}

// A namespace that is not a peer is never called, whatever the registry holds:
// the registry also holds services that are not storage peers.
func TestFiles_Locate_NonPeerIsNeverCalled(t *testing.T) {
	p := newPeer(t, answerWith(http.StatusOK, goodAnswer("https://c.example/x")))
	registry := peerRegistry(t, "slpa", p.URL)
	f := NewFiles(ownService{}, registry, map[string]string{"cda": p.URL}, nil)

	ref, err := fileref.Foreign("slpa", token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.locate(context.Background(), ref.String()); !isUpstreamFailure(err) {
		t.Fatalf("err = %v, want an upstream failure", err)
	}
	if p.hits() != 0 {
		t.Errorf("a service that is not a peer was called %d times", p.hits())
	}
}

// Without peers, a value that looks foreign is just another value of this
// deployment, which storage refuses.
func TestFiles_NoPeers(t *testing.T) {
	f := NewFiles(ownService{}, nil, nil, nil)
	if _, err := f.locate(context.Background(), foreign(t)); !isUpstreamFailure(err) {
		t.Fatalf("err = %v, want an upstream failure", err)
	}
	if _, _, err := f.Download(context.Background(), foreign(t)); !isUpstreamFailure(err) {
		t.Fatalf("Download err = %v, want an upstream failure", err)
	}
}

func TestFiles_Download_OwnValue(t *testing.T) {
	f := NewFiles(ownService{}, nil, nil, nil)
	body, contentType, err := f.Download(context.Background(), "3f2b9c1e-0a4d-4b7e-9c11-5d6e7f8a9b0c.pdf")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = body.Close() }()
	if got, _ := io.ReadAll(body); string(got) != "own bytes" || contentType != "application/pdf" {
		t.Errorf("Download = %q, %q", got, contentType)
	}
}

// A server-side read fetches the owner's file without this deployment's
// credentials, from the owner's own storage.
func TestFiles_Download_ForeignValue(t *testing.T) {
	var content *peer
	content = newPeer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			if r.Header.Get("Authorization") != "" {
				t.Errorf("the presigned URL was fetched with credentials: %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF-peer")
			return
		}
		answerWith(http.StatusOK, goodAnswer(content.URL+"/api/v1/storage/k/content?sig=1"))(w, r)
	})
	f := newPeerFiles(t, content)

	body, contentType, err := f.Download(context.Background(), foreign(t))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = body.Close() }()
	if got, _ := io.ReadAll(body); string(got) != "%PDF-peer" || contentType != "application/pdf" {
		t.Errorf("Download = %q, %q", got, contentType)
	}
}

// The URL an owner gives a server-side read is the owner's to choose, so the
// read refuses what it should not reach.
func TestFiles_Download_RefusesWhereItShouldNotGo(t *testing.T) {
	internal := newPeer(t, answerWith(http.StatusOK, "internal secret"))
	var target atomic.Value // the URL the owner gives, set by each case below
	api := newPeer(t, func(w http.ResponseWriter, r *http.Request) {
		answerWith(http.StatusOK, goodAnswer(target.Load().(string)))(w, r)
	})
	f := NewFiles(ownService{}, peerRegistry(t, "cda", api.URL), map[string]string{"cda": api.URL}, nil)

	tests := []struct {
		name, target string
	}{
		{"another loopback server over http", internal.URL + "/x"},
		{"loopback over https", "https://127.0.0.1:1/x"},
		{"a private address", "https://10.0.0.5/x"},
		{"the metadata address", "http://169.254.169.254/latest/meta-data/"},
		{"the metadata address over https", "https://169.254.169.254/x"},
		{"an IPv6 loopback", "https://[::1]/x"},
		{"an IPv4-mapped private address", "https://[::ffff:10.0.0.5]/x"},
		{"a unique-local address", "https://[fc00::1]/x"},
		{"a name that is loopback", "https://localhost/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target.Store(tt.target)
			body, _, err := f.Download(context.Background(), foreign(t))
			if err == nil {
				_ = body.Close()
				t.Fatal("Download succeeded")
			}
			if !isUpstreamFailure(err) {
				t.Errorf("err = %v, want an upstream failure", err)
			}
			if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "169.254") || strings.Contains(err.Error(), "10.0.0.5") {
				t.Errorf("err %q names the URL", err)
			}
		})
	}
	if internal.hits() != 0 {
		t.Errorf("the internal server was reached %d times", internal.hits())
	}
}

// A redirect from the owner's storage is not followed.
func TestFiles_Download_DoesNotFollowRedirects(t *testing.T) {
	internal := newPeer(t, answerWith(http.StatusOK, "internal secret"))
	var api *peer
	api = newPeer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			http.Redirect(w, r, internal.URL+"/secret", http.StatusFound)
			return
		}
		answerWith(http.StatusOK, goodAnswer(api.URL+"/api/v1/storage/k/content"))(w, r)
	})
	f := newPeerFiles(t, api)

	if body, _, err := f.Download(context.Background(), foreign(t)); err == nil {
		_ = body.Close()
		t.Fatal("Download followed a redirect")
	}
	if internal.hits() != 0 {
		t.Errorf("the redirect target was reached %d times", internal.hits())
	}
}

func TestPublicAddress(t *testing.T) {
	tests := map[string]bool{
		"8.8.8.8":              true,
		"1.1.1.1":              true,
		"2001:4860:4860::8888": true,
		"127.0.0.1":            false,
		"::1":                  false,
		"10.1.2.3":             false,
		"172.16.0.1":           false,
		"192.168.1.1":          false,
		"169.254.169.254":      false,
		"fe80::1":              false,
		"fc00::1":              false,
		"100.64.0.1":           false,
		"0.0.0.0":              false,
		"::":                   false,
		"224.0.0.1":            false,
		"198.18.0.1":           false,
		"240.0.0.1":            false,
		"::ffff:10.0.0.1":      false,
		"::ffff:8.8.8.8":       true,
	}
	for addr, want := range tests {
		if got := publicAddress(netip.MustParseAddr(addr)); got != want {
			t.Errorf("publicAddress(%s) = %v, want %v", addr, got, want)
		}
	}
}

// The registered host of a peer may serve its own storage over http, as a
// deployment on local disk does.
func TestFiles_Download_PeerHostMayBePrivateHTTP(t *testing.T) {
	var api *peer
	api = newPeer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			_, _ = io.WriteString(w, "from the peer")
			return
		}
		answerWith(http.StatusOK, goodAnswer(api.URL+"/api/v1/storage/k/content"))(w, r)
	})
	f := newPeerFiles(t, api)
	body, _, err := f.Download(context.Background(), foreign(t))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = body.Close() }()
	if got, _ := io.ReadAll(body); string(got) != "from the peer" {
		t.Errorf("body = %q", got)
	}
}

// A foreign file is opened through the handler as through Files: the caller
// holds a token for the stored value, and gets the owner's URL.
func TestFileHandler_DownloadForeignValue(t *testing.T) {
	p := newPeer(t, answerWith(http.StatusOK, goodAnswer("https://cdn.example/x?sig=1")))
	access, _ := testFileAccess(t)
	h := NewFileHandler(newPeerFiles(t, p), access)

	ref, err := access.Issue(user("alice"), foreign(t), fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	rec := download(h, ref, user("alice"))
	assertStatus(t, rec, http.StatusOK)
	assertFileType(t, rec, ".pdf", "application/pdf", "inline")
	if !strings.Contains(rec.Body.String(), "https://cdn.example/x?sig=1") {
		t.Errorf("body %s lacks the owner's URL", rec.Body.String())
	}

	// Someone else's token resolves nothing, and the owner is never asked.
	hits := p.hits()
	assertStatus(t, download(h, ref, user("bob")), http.StatusForbidden)
	if p.hits() != hits {
		t.Error("the owner was asked for a file the caller holds no token for")
	}
}

func TestFileHandler_DownloadForeignValue_OwnerFailures(t *testing.T) {
	access, _ := testFileAccess(t)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	notFound := NewFileHandler(newPeerFiles(t, newPeer(t, answerWith(http.StatusNotFound, `{"error":"secret owner text"}`))), access)
	failing := NewFileHandler(newPeerFiles(t, newPeer(t, answerWith(http.StatusInternalServerError, `{"error":"secret owner text"}`))), access)
	ref, err := access.Issue(user("alice"), foreign(t), fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}

	rec := download(notFound, ref, user("alice"))
	assertStatus(t, rec, http.StatusNotFound)
	rec = download(failing, ref, user("alice"))
	assertStatus(t, rec, http.StatusBadGateway)
	for _, body := range []string{rec.Body.String(), download(notFound, ref, user("alice")).Body.String()} {
		if strings.Contains(body, "secret owner text") || strings.Contains(body, "cda") || strings.Contains(body, token) {
			t.Errorf("response %s names the owner, its text or the token", body)
		}
	}
}
