package bootstrap

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	nswaudit "github.com/OpenNSW/core/platform/audit"
	"github.com/OpenNSW/core/platform/authn"
	"github.com/OpenNSW/core/platform/catalog"
	nswstorage "github.com/OpenNSW/core/platform/storage"
	"github.com/OpenNSW/core/platform/storage/fileaccess"
	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// A proxy serves no internal upload route, so it reads none.
	proxy := nswstorage.Config{AllowedInternalUploadTypes: map[string][]string{"nobody": {"text/html"}}}
	proxy.Type = nswstorage.TypeProxy
	if got, err := internalUploadTypesByClient(proxy, cat); err != nil || len(got) != 0 {
		t.Errorf("in proxy mode: %v, %v, want an empty map", got, err)
	}
}

func newTestStorageHandler(t *testing.T, auditor nswaudit.Auditor, opts ...nswstorage.FileHandlerOption) (*nswstorage.Stack, *fileaccess.Access, *nswstorage.FileHandler) {
	t.Helper()
	cfg := nswstorage.Config{
		Config: corestorage.Config{
			Type: corestorage.TypeLocal,
			Local: drivers.LocalConfig{
				BaseDir:   t.TempDir(),
				PublicURL: "http://storage.test",
				PutSecret: "secret",
			},
			PresignTTLSeconds: 900,
		},
		AllowedUploadTypes: []string{"application/pdf"},
		UploadTypeExtensions: map[string][]string{
			"application/pdf": {".pdf"},
			"text/html":       {".html"},
		},
		InlineTypes:    corestorage.DefaultInlineTypes(),
		MaxUploadBytes: 1 << 20,
		TokenKeyset:    "test:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
	}
	stack, err := nswstorage.New(t.Context(), cfg, nil, nswstorage.WithInternalUploadTypes(map[string][]string{
		"NPQS_TO_NSW": {"text/html"},
	}))
	require.NoError(t, err)
	access, err := newFileAccess(cfg)
	require.NoError(t, err)
	return stack, access, newFileHandler(stack, access, auditor, opts...)
}

func TestNewFileHandler_WiresUploadersAndAuditor(t *testing.T) {
	auditor, capture := nswaudit.NewWithCapture()
	stack, access, h := newTestStorageHandler(t, auditor)

	alice := &authn.Principal{Kind: authn.KindUser, IDPUserID: "alice"}
	agency := &authn.Principal{Kind: authn.KindClient, ClientID: "NPQS_TO_NSW"}

	withPrincipal := func(req *http.Request, principal *authn.Principal) *http.Request {
		return req.WithContext(authn.ContextWithPrincipal(req.Context(), principal))
	}

	uploadReq := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/storage", strings.NewReader(`{"filename":"invoice.pdf","mime_type":"application/pdf","size":12}`)), alice)
	uploadRec := httptest.NewRecorder()
	h.Upload(uploadRec, uploadReq)
	require.Equal(t, http.StatusOK, uploadRec.Code, uploadRec.Body.String())

	machineReq := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/internal/storage", strings.NewReader(`{"filename":"cert.html","mime_type":"text/html","size":24}`)), agency)
	machineRec := httptest.NewRecorder()
	h.InternalUpload(machineRec, machineReq)
	require.Equal(t, http.StatusOK, machineRec.Code, machineRec.Body.String())

	saved, err := stack.Service.Save(t.Context(), "doc.pdf", "application/pdf", strings.NewReader("pdf-bytes"), int64(len("pdf-bytes")))
	require.NoError(t, err)
	readRef, err := access.Issue(alice, saved.Key, fileaccess.ScopeRead)
	require.NoError(t, err)

	downloadReq := withPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/storage/"+url.PathEscape(readRef), nil), alice)
	downloadReq.SetPathValue("key", readRef)
	downloadRec := httptest.NewRecorder()
	h.Download(downloadRec, downloadReq)
	require.Equal(t, http.StatusOK, downloadRec.Code, downloadRec.Body.String())

	recs := capture.Records()
	require.Len(t, recs, 3)
	assert.Equal(t, nswaudit.ActionPresignUpload, recs[0].Action)
	assert.Equal(t, nswaudit.ActionPresignUpload, recs[1].Action)
	assert.Equal(t, nswaudit.ActionRead, recs[2].Action)
}
