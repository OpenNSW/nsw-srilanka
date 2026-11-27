package bootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenNSW/core/httputil"
	nswaudit "github.com/OpenNSW/core/platform/audit"
	"github.com/OpenNSW/core/platform/authn"
	"github.com/OpenNSW/core/platform/catalog"
	nswstorage "github.com/OpenNSW/core/platform/storage"
	"github.com/OpenNSW/core/platform/storage/fileaccess"
	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
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

type otlpLogCollector struct {
	server *httptest.Server

	mu   sync.Mutex
	logs [][]byte
	path string
	auth string
}

func newOTLPLogCollector(t *testing.T) *otlpLogCollector {
	t.Helper()
	c := &otlpLogCollector{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if strings.HasSuffix(r.URL.Path, "/v1/logs") {
			c.mu.Lock()
			c.logs = append(c.logs, body)
			c.path = r.URL.Path
			c.auth = r.Header.Get("Authorization")
			c.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.server.Close)
	return c
}

func (c *otlpLogCollector) snapshot() (payload []byte, path, auth string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, body := range c.logs {
		payload = append(payload, body...)
	}
	return payload, c.path, c.auth
}

func newTestStorageHandler(t *testing.T, auditor nswaudit.Auditor) (*nswstorage.Stack, *fileaccess.Access, *nswstorage.FileHandler) {
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
	return stack, access, newFileHandler(stack, access, auditor)
}

func sha256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func TestNewFileHandler_AuditsUploadAndDownloadOverOTLP(t *testing.T) {
	const auth = "Basic dGVzdDp0ZXN0"
	collector := newOTLPLogCollector(t)

	prevDefault := slog.Default()
	prevCorrelation := httputil.CorrelationIDFunc
	prevTracer := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		httputil.CorrelationIDFunc = prevCorrelation
		otel.SetTracerProvider(prevTracer)
		otel.SetTextMapPropagator(prevProp)
	})

	p, err := telemetry.Start(t.Context(), telemetry.Config{
		Endpoint:    collector.server.URL + "/api/default",
		ServiceName: "tnsw-api",
		Headers:     map[string]string{"Authorization": auth},
	})
	require.NoError(t, err)

	appCfg := &config.Config{Server: config.ServerConfig{LogLevel: slog.LevelError}}
	var process bytes.Buffer
	configureLogging(appCfg, &process, p.SlogHandler())

	auditor, capture := nswaudit.NewWithCapture()
	auditor.RegisterSink(nswaudit.NewLogSink(slog.New(auditExportHandler(appCfg, p.SlogHandler()))))

	stack, access, h := newTestStorageHandler(t, auditor)
	alice := &authn.Principal{Kind: authn.KindUser, IDPUserID: "alice"}
	bob := &authn.Principal{Kind: authn.KindUser, IDPUserID: "bob"}
	agency := &authn.Principal{Kind: authn.KindClient, ClientID: "NPQS_TO_NSW"}

	// User upload prepare.
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/storage", strings.NewReader(`{"filename":"invoice.pdf","mime_type":"application/pdf","size":12}`))
	uploadReq = uploadReq.WithContext(authn.ContextWithPrincipal(uploadReq.Context(), alice))
	uploadRec := httptest.NewRecorder()
	h.Upload(uploadRec, uploadReq)
	require.Equal(t, http.StatusOK, uploadRec.Code, uploadRec.Body.String())

	var uploadResp struct {
		Key string `json:"key"`
	}
	require.NoError(t, json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp))
	userRefSHA := sha256Hex(uploadResp.Key)

	// Machine upload prepare.
	machineReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/storage", strings.NewReader(`{"filename":"cert.html","mime_type":"text/html","size":24}`))
	machineReq = machineReq.WithContext(authn.ContextWithPrincipal(machineReq.Context(), agency))
	machineRec := httptest.NewRecorder()
	h.InternalUpload(machineRec, machineReq)
	require.Equal(t, http.StatusOK, machineRec.Code, machineRec.Body.String())

	// Download link issued and refused for a stored object.
	saved, err := stack.Service.Save(t.Context(), "doc.pdf", "application/pdf", strings.NewReader("pdf-bytes"), int64(len("pdf-bytes")))
	require.NoError(t, err)
	readRef, err := access.Issue(alice, saved.Key, fileaccess.ScopeRead)
	require.NoError(t, err)
	readRefSHA := sha256Hex(readRef)

	callDownload := func(principal *authn.Principal, ref string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/storage/"+url.PathEscape(ref), nil)
		req.SetPathValue("key", ref)
		req = req.WithContext(authn.ContextWithPrincipal(req.Context(), principal))
		rec := httptest.NewRecorder()
		h.Download(rec, req)
		return rec
	}
	require.Equal(t, http.StatusOK, callDownload(alice, readRef).Code)
	require.Equal(t, http.StatusForbidden, callDownload(bob, readRef).Code)

	recs := capture.Records()
	require.Len(t, recs, 4)
	assert.Equal(t, nswaudit.EventStorage, recs[0].EventType)
	assert.Equal(t, nswaudit.ActionPresignUpload, recs[0].Action)
	assert.Equal(t, nswaudit.StatusSuccess, recs[0].Status)
	assert.NotEmpty(t, recs[0].TargetID)
	assert.Equal(t, userRefSHA, recs[0].Metadata["reference_sha256"])

	assert.Equal(t, nswaudit.ActionPresignUpload, recs[1].Action)
	assert.Equal(t, nswaudit.StatusSuccess, recs[1].Status)
	assert.Equal(t, "NPQS_TO_NSW", recs[1].ActorID)

	assert.Equal(t, nswaudit.ActionRead, recs[2].Action)
	assert.Equal(t, nswaudit.StatusSuccess, recs[2].Status)
	assert.Equal(t, saved.Key, recs[2].TargetID)
	assert.Equal(t, readRefSHA, recs[2].Metadata["reference_sha256"])

	assert.Equal(t, nswaudit.ActionRead, recs[3].Action)
	assert.Equal(t, nswaudit.StatusFailure, recs[3].Status)
	assert.Empty(t, recs[3].TargetID)
	assert.Equal(t, "not_yours", recs[3].Metadata["reason"])

	// Flush OTLP logs and verify the audit records reached the collector even
	// at error log level, without leaking into the process log.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, p.Shutdown(ctx))

	got, gotPath, gotAuth := collector.snapshot()
	require.Equal(t, "/api/default/v1/logs", gotPath)
	require.Equal(t, auth, gotAuth)
	for _, want := range []string{
		"STORAGE_EVENT",
		"PRESIGN_UPLOAD",
		"READ",
		"STORAGE_OBJECT",
		saved.Key,
		userRefSHA,
		readRefSHA,
		"not_yours",
	} {
		assert.Contains(t, string(got), want)
	}
	assert.NotContains(t, string(got), readRef)
	assert.NotContains(t, string(got), uploadResp.Key)
	assert.NotContains(t, process.String(), "STORAGE_EVENT")
	assert.NotContains(t, process.String(), "category=audit")
}
