package documents

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pdf is enough of a PDF for content sniffing to call it one.
var pdf = []byte("%PDF-1.4\n%stub\n")

type stored struct {
	filename string
	content  []byte
	mime     string
}

type fakeStore struct {
	files []stored
	err   error
}

func (s *fakeStore) Store(_ context.Context, filename string, content []byte, mime string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.files = append(s.files, stored{filename, content, mime})
	return fmt.Sprintf("00000000-0000-0000-0000-%012d%s", len(s.files), filepath.Ext(filename)), nil
}

// servicesFor returns a remote manager whose service "slpa" is srv, so the
// tests go through the real client — and its same-host rule — rather than a
// fake of it.
func servicesFor(t *testing.T, srv *httptest.Server) *remote.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "services.json")
	registry := fmt.Sprintf(`{"services":[{"id":"slpa","url":%q,"timeout":"5s"}]}`, srv.URL)
	require.NoError(t, os.WriteFile(path, []byte(registry), 0o600))
	m := remote.NewManager()
	require.NoError(t, m.LoadServices(path))
	return m
}

func TestArchiveFieldsReplacesLinkWithStorageKey(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	defer srv.Close()
	store := &fakeStore{}
	a := NewArchiver(store, servicesFor(t, srv))

	out := map[string]any{"payment_slip_url": srv.URL + "/pdf/payment-slip/2621?signature=18ce9a02", "invoice_no": "2621"}
	a.ArchiveFields(context.Background(), "slpa", out, []Field{{Key: "payment_slip_url", Name: "slpa-payment-slip"}})

	assert.Equal(t, "00000000-0000-0000-0000-000000000001.pdf", out["payment_slip_url"])
	assert.Equal(t, "2621", out["invoice_no"], "fields not named are left alone")
	assert.Equal(t, "signature=18ce9a02", gotQuery, "the signature is what authorises the fetch, so it must reach the host")
	require.Len(t, store.files, 1)
	assert.Equal(t, stored{"slpa-payment-slip.pdf", pdf, "application/pdf"}, store.files[0])
}

func TestArchiveFieldsStoresAnInlineBarcode(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nstub")
	store := &fakeStore{}
	a := NewArchiver(store, remote.NewManager())

	out := map[string]any{"barcode": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}
	a.ArchiveFields(context.Background(), "slpa", out, []Field{{Key: "barcode", Name: "slpa-gate-pass-barcode"}})

	assert.Equal(t, "00000000-0000-0000-0000-000000000001.png", out["barcode"])
	require.Len(t, store.files, 1)
	assert.Equal(t, stored{"slpa-gate-pass-barcode.png", png, "image/png"}, store.files[0])
}

func TestArchiveFieldsSniffsAnUnlabelledDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(pdf)
	}))
	defer srv.Close()
	store := &fakeStore{}
	a := NewArchiver(store, servicesFor(t, srv))

	out := map[string]any{"gate_pass_url": srv.URL + "/pdf/gate-pass/756895"}
	a.ArchiveFields(context.Background(), "slpa", out, []Field{{Key: "gate_pass_url", Name: "slpa-gate-pass"}})

	assert.Equal(t, "00000000-0000-0000-0000-000000000001.pdf", out["gate_pass_url"])
	assert.Equal(t, "application/pdf", store.files[0].mime)
}

// Each of these leaves the trader with the provider's link: the step that
// issued it has succeeded, and a link that may expire beats no document.
func TestArchiveFieldsKeepsTheLinkWhenTheDocumentCannotBeKept(t *testing.T) {
	cases := map[string]struct {
		handler  http.HandlerFunc
		storeErr error
		link     func(srvURL string) string
	}{
		"expired signature": {
			handler: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "signature expired", http.StatusForbidden) },
		},
		"web page served as 200": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte("<html>please log in</html>"))
			},
		},
		"empty body": {
			handler: func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("Content-Type", "application/pdf") },
		},
		"storage down": {
			handler:  func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(pdf) },
			storeErr: errors.New("bucket unreachable"),
		},
		"link on another host": {
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(pdf) },
			link:    func(string) string { return "http://169.254.169.254/latest/meta-data?signature=x" },
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			link := srv.URL + "/pdf/payment-slip/2621?signature=18ce9a02"
			if tc.link != nil {
				link = tc.link(srv.URL)
			}
			a := NewArchiver(&fakeStore{err: tc.storeErr}, servicesFor(t, srv))

			out := map[string]any{"payment_slip_url": link}
			a.ArchiveFields(context.Background(), "slpa", out, []Field{{Key: "payment_slip_url", Name: "slpa-payment-slip"}})

			assert.Equal(t, link, out["payment_slip_url"])
		})
	}
}

func TestArchiveFieldsLeavesStorageKeysAndBlanksAlone(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	store := &fakeStore{}
	a := NewArchiver(store, servicesFor(t, srv))

	const key = "3f2b6c1e-8a4d-4e1f-9b7c-2d5e8f1a3b6c.pdf"
	out := map[string]any{"gate_pass_url": key, "barcode": ""}
	a.ArchiveFields(context.Background(), "slpa", out, []Field{{Key: "gate_pass_url"}, {Key: "barcode"}, {Key: "absent"}})

	assert.Equal(t, key, out["gate_pass_url"], "an archived answer archived again is unchanged")
	assert.Equal(t, "", out["barcode"])
	assert.NotContains(t, out, "absent")
	assert.False(t, called)
	assert.Empty(t, store.files)
}

func TestArchiveErrorsNeverCarryTheSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()
	a := NewArchiver(&fakeStore{}, servicesFor(t, srv))

	_, err := a.Archive(context.Background(), "slpa", srv.URL+"/pdf/payment-receipt/1?signature=secret-sig", "receipt")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-sig")

	srv.Close() // a transport failure quotes the URL it failed on
	_, err = a.Archive(context.Background(), "slpa", srv.URL+"/pdf/payment-receipt/1?signature=secret-sig", "receipt")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-sig")
}

func TestArchiveRefusesWhatIsNotALink(t *testing.T) {
	a := NewArchiver(&fakeStore{}, remote.NewManager())
	for _, src := range []string{"file:///etc/passwd", "data:image/png,not-base64", "slip.pdf"} {
		_, err := a.Archive(context.Background(), "slpa", src, "doc")
		assert.Error(t, err, src)
	}
}
