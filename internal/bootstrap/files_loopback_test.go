package bootstrap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenNSW/core/remote"
	remoteauth "github.com/OpenNSW/core/remote/auth"
	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks"
)

// deployment is one running deployment of the application's file services,
// reachable over HTTP by the peers its catalog names.
type deployment struct {
	t       *testing.T
	srv     *httptest.Server
	files   *fileServices
	bucket  string
	clients map[string]*authn.Principal // bearer token -> the machine client it stands for
}

// newDeployment starts a deployment whose token keys are filled with seed, so
// that no two deployments share a keyset. peers are the services it calls, each
// with the bearer token it calls it by; accepts says which machine client each
// bearer token the deployment accepts stands for.
func newDeployment(t *testing.T, seed byte, cat *catalog.Catalog, peers map[string]*deployment, bearers map[string]string, accepts map[string]string) *deployment {
	t.Helper()
	d := &deployment{t: t, bucket: t.TempDir(), clients: map[string]*authn.Principal{}}
	for token, clientID := range accepts {
		d.clients[token] = &authn.Principal{Kind: authn.KindClient, ClientID: clientID}
	}

	var handler http.Handler
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(d.srv.Close)

	var services []remote.ServiceConfig
	for id, peer := range peers {
		services = append(services, remote.ServiceConfig{
			ID: id, URL: peer.srv.URL,
			Auth: &remoteauth.Config{Type: remoteauth.TypeBearer, Bearer: &remoteauth.BearerConfig{Token: bearers[id]}},
		})
	}
	rm := remote.NewManager()
	require.NoError(t, rm.SetServices(services...))

	keyset := "k1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 32))
	cfg := nswstorage.Config{
		Config: corestorage.Config{
			Type:              corestorage.TypeLocal,
			PresignTTLSeconds: 900,
			Local:             drivers.LocalConfig{BaseDir: d.bucket, PublicURL: d.srv.URL, PutSecret: "secret-" + t.Name()},
		},
		AllowedUploadTypes: []string{"application/pdf"},
		InlineTypes:        []string{"application/pdf"},
		MaxUploadBytes:     1 << 20,
		TokenKeyset:        keyset,
	}
	files, err := newFileServices(context.Background(), cfg, services, cat, rm)
	require.NoError(t, err)
	d.files = files

	mux := http.NewServeMux()
	h := nswstorage.NewFileHandler(files.Files, files.Access)
	mux.HandleFunc(nswstorage.UploadRoute, d.asClient(h.Upload))
	mux.HandleFunc(nswstorage.DownloadRoute, d.asClient(h.Download))
	files.Stack.LocalContent.RegisterRoutes(mux)
	handler = mux
	return d
}

// asClient authenticates a request by its bearer token, as the real middleware
// does for a machine client.
func (d *deployment) asClient(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := d.clients[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(authn.ContextWithPrincipal(r.Context(), p)))
	}
}

// bucketFiles counts the files this deployment stores.
func (d *deployment) bucketFiles() int {
	n := 0
	_ = filepath.WalkDir(d.bucket, func(_ string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func (d *deployment) as(p *authn.Principal, req *http.Request) *http.Request {
	return req.WithContext(authn.ContextWithPrincipal(req.Context(), p))
}

// upload reserves an upload as p and puts content to it, and returns the token p
// is given for the file.
func (d *deployment) upload(p *authn.Principal, content string) string {
	d.t.Helper()
	req := d.as(p, httptest.NewRequest(http.MethodPost, "/api/v1/storage",
		strings.NewReader(`{"filename":"certificate.pdf","mime_type":"application/pdf","size":`+itoa(len(content))+`}`)))
	rec := httptest.NewRecorder()
	nswstorage.NewFileHandler(d.files.Files, d.files.Access).Upload(rec, req)
	require.Equal(d.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Key       string `json:"key"`
		UploadURL string `json:"upload_url"`
	}
	require.NoError(d.t, json.Unmarshal(rec.Body.Bytes(), &resp))

	put, err := http.NewRequest(http.MethodPut, resp.UploadURL, strings.NewReader(content))
	require.NoError(d.t, err)
	put.Header.Set("Content-Type", "application/pdf")
	res, err := http.DefaultClient.Do(put)
	require.NoError(d.t, err)
	_ = res.Body.Close()
	require.Less(d.t, res.StatusCode, 300)
	return resp.Key
}

// open opens a file as p with token and returns what a browser gets from the
// URL, or the status the deployment answered when it gave none.
func (d *deployment) open(p *authn.Principal, token string) (body string, status int) {
	d.t.Helper()
	req := d.as(p, httptest.NewRequest(http.MethodGet, "/api/v1/storage/"+token, nil))
	req.SetPathValue("key", token)
	rec := httptest.NewRecorder()
	nswstorage.NewFileHandler(d.files.Files, d.files.Access).Download(rec, req)
	if rec.Code != http.StatusOK {
		return "", rec.Code
	}
	var resp struct {
		DownloadURL string `json:"download_url"`
	}
	require.NoError(d.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	res, err := http.Get(resp.DownloadURL)
	require.NoError(d.t, err)
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	require.NoError(d.t, err)
	require.Equal(d.t, http.StatusOK, res.StatusCode)
	return string(b), http.StatusOK
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

const (
	tnswToCDAToken = "tnsw-calls-cda"
	cdaToTNSWToken = "cda-calls-tnsw"
)

// twoDeployments starts TNSW and the CDA, each the other's storage peer, as in
// the file flows: TNSW knows CDA_TO_NSW as cda, and the CDA knows NSW_TO_CDA as
// tnsw.
func twoDeployments(t *testing.T) (tnsw, cda *deployment) {
	t.Helper()
	// Each needs the other's address, so the CDA starts first with a stand-in
	// peer that is replaced below by registering TNSW once it is up.
	tnswCatalog := &catalog.Catalog{
		Clients:         map[string]string{"cda": "CDA_TO_NSW"},
		ClientToService: map[string]string{"cda": "cda"},
	}
	cdaCatalog := &catalog.Catalog{
		Roles:           map[string]string{"officer": "CDA Officer"},
		Clients:         map[string]string{"tnsw": "NSW_TO_CDA"},
		ClientToService: map[string]string{"tnsw": "tnsw"},
	}
	// The CDA is built first, calling a TNSW whose URL is filled in afterwards
	// through a forwarding server.
	var tnswHandler http.Handler
	forward := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tnswHandler.ServeHTTP(w, r) }))
	t.Cleanup(forward.Close)

	cda = newDeployment(t, 2, cdaCatalog, map[string]*deployment{"tnsw": {srv: forward}}, map[string]string{"tnsw": cdaToTNSWToken}, map[string]string{tnswToCDAToken: "NSW_TO_CDA"})
	tnsw = newDeployment(t, 1, tnswCatalog, map[string]*deployment{"cda": cda}, map[string]string{"cda": tnswToCDAToken}, map[string]string{cdaToTNSWToken: "CDA_TO_NSW"})
	tnswHandler = tnsw.srv.Config.Handler
	return tnsw, cda
}

var (
	officer = &authn.Principal{Kind: authn.KindUser, IDPUserID: "officer-1"}
	trader  = &authn.Principal{Kind: authn.KindUser, IDPUserID: "trader-1"}
	other   = &authn.Principal{Kind: authn.KindUser, IDPUserID: "trader-2"}
	cdaAt   = &authn.Principal{Kind: authn.KindClient, ClientID: "CDA_TO_NSW"}
	tnswAt  = &authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA"}
)

func renderWith(files ...string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"files": files})
	return b
}

func namespaceOf(ns string) tasks.NamespaceOf {
	return func(context.Context, string) (string, error) { return ns, nil }
}

// An officer's upload stays in the agency's storage. TNSW stores the token the
// agency gives it as the agency's file, and a trader opens it through the agency.
func TestFileServices_AgencyFileOpenedByATraderThroughTNSW(t *testing.T) {
	tnsw, cda := twoDeployments(t)

	// The officer uploads to the agency, submits the form, and the decision step
	// hands TNSW's client a token for the stored key.
	officerToken := cda.upload(officer, "%PDF-certificate")
	key, err := cda.files.Access.Accept(officer, officerToken)
	require.NoError(t, err)
	assert.Equal(t, 0, tnsw.bucketFiles())
	assert.NotZero(t, cda.bucketFiles())
	client, ok := cda.files.OGA.ClientFor("tnsw")
	require.True(t, ok)
	forTNSW, err := cda.files.OGA.IssueForClient(client, key)
	require.NoError(t, err)

	// TNSW's callback intake stores it from the authenticated client.
	binding := tasks.NewFileBinding(tnsw.files.Access, namespaceOf("reviewerform"))
	record := store.TaskRecord{RenderConfig: renderWith("reviewerform.certificate"), ActiveTaskTemplateID: "review"}
	payload := map[string]any{"certificate": forTNSW}
	require.NoError(t, binding.FromCaller(context.Background(), record, payload, cdaAt))
	stored, _ := payload["certificate"].(string)
	assert.True(t, strings.HasPrefix(stored, "cda:"), "stored %q is not the agency's file", stored)

	// The trader's view of the task holds a token for it, which opens the file.
	record.Data = map[string]any{"reviewerform": map[string]any{"certificate": stored}}
	view, err := binding.ForReader(record, trader, false)
	require.NoError(t, err)
	traderToken := view.Data["reviewerform"].(map[string]any)["certificate"].(string)

	body, status := tnsw.open(trader, traderToken)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "%PDF-certificate", body)
	assert.Equal(t, 0, tnsw.bucketFiles(), "TNSW holds a copy of the agency's file")

	// The token is the trader's alone.
	_, status = tnsw.open(other, traderToken)
	assert.Equal(t, http.StatusForbidden, status)
}

// A file TNSW pushes is stored by the agency as TNSW's, and an officer opens it
// through TNSW. TNSW's token is read only, so the agency cannot send it back.
func TestFileServices_TNSWFileOpenedByAnOfficerThroughTheAgency(t *testing.T) {
	tnsw, cda := twoDeployments(t)

	traderToken := tnsw.upload(trader, "%PDF-invoice")
	key, err := tnsw.files.Access.Accept(trader, traderToken)
	require.NoError(t, err)
	client, ok := tnsw.files.OGA.ClientFor("cda")
	require.True(t, ok)
	pushed, err := tnsw.files.OGA.IssueForClient(client, key)
	require.NoError(t, err)

	// The agency's inject stores the token the injecting client sent.
	stored, err := cda.files.Access.Accept(tnswAt, pushed)
	require.NoError(t, err)
	assert.Equal(t, "tnsw:"+pushed, stored)

	binding := tasks.NewFileBinding(cda.files.Access, namespaceOf("submission"))
	record := store.TaskRecord{
		RenderConfig: renderWith("submission.invoice"), ActiveTaskTemplateID: "review",
		Data: map[string]any{"submission": map[string]any{"invoice": stored}},
	}
	view, err := binding.ForReader(record, officer, true)
	require.NoError(t, err)
	officerToken := view.Data["submission"].(map[string]any)["invoice"].(string)

	body, status := cda.open(officer, officerToken)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "%PDF-invoice", body)
	assert.Equal(t, 0, cda.bucketFiles(), "the agency holds a copy of TNSW's file")

	// The agency cannot send TNSW's token back into a TNSW task.
	_, err = tnsw.files.Access.Accept(&authn.Principal{Kind: authn.KindClient, ClientID: "CDA_TO_NSW"}, pushed)
	assert.ErrorIs(t, err, fileaccess.ErrScope)
}

// A file the agency produced, pushed back to the agency by TNSW, is opened by
// the officer through TNSW and then through the agency again.
func TestFileServices_FileReturnsToTheServiceThatProducedIt(t *testing.T) {
	tnsw, cda := twoDeployments(t)

	key, err := cda.files.Access.Accept(officer, cda.upload(officer, "%PDF-letter"))
	require.NoError(t, err)
	client, _ := cda.files.OGA.ClientFor("tnsw")
	forTNSW, err := cda.files.OGA.IssueForClient(client, key)
	require.NoError(t, err)
	storedAtTNSW, err := tnsw.files.Access.Accept(cdaAt, forTNSW)
	require.NoError(t, err)

	// TNSW pushes it back to the agency, wrapped in a token of its own.
	toCDA, _ := tnsw.files.OGA.ClientFor("cda")
	pushed, err := tnsw.files.OGA.IssueForClient(toCDA, storedAtTNSW)
	require.NoError(t, err)
	storedAtCDA, err := cda.files.Access.Accept(tnswAt, pushed)
	require.NoError(t, err)

	officerToken, err := cda.files.Access.Issue(officer, storedAtCDA, fileaccess.ScopeRead)
	require.NoError(t, err)
	body, status := cda.open(officer, officerToken)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "%PDF-letter", body)
}

// An agency that is not a peer of the deployment cannot make it call anything.
func TestFileServices_NonPeerFilesAreNotOpened(t *testing.T) {
	tnsw, _ := twoDeployments(t)
	token, err := tnsw.files.Access.Issue(trader, "slpa:eyJhbGciOiJkaXIifQ..AAAA.BBBB.CCCC", fileaccess.ScopeRead)
	require.NoError(t, err)
	_, status := tnsw.open(trader, token)
	assert.Equal(t, http.StatusBadGateway, status)
}

func TestStoragePeers_Validates(t *testing.T) {
	rm := remote.NewManager()
	services := []remote.ServiceConfig{{ID: "cda", URL: "http://cda.example"}, {ID: "slpa", URL: "http://slpa.example"}}
	require.NoError(t, rm.SetServices(services...))

	peers, err := storagePeers(&catalog.Catalog{ClientToService: map[string]string{"cda": "cda"}}, services, rm)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"cda": "http://cda.example"}, peers)

	_, err = storagePeers(&catalog.Catalog{ClientToService: map[string]string{"x": "fcau"}}, services, rm)
	assert.ErrorContains(t, err, `service "fcau" is not in the services registry`)

	_, err = storagePeers(&catalog.Catalog{ClientToService: map[string]string{"x": "a:b"}}, services, rm)
	assert.ErrorContains(t, err, "cannot name a service")

	peers, err = storagePeers(&catalog.Catalog{}, services, rm)
	require.NoError(t, err)
	assert.Empty(t, peers)
}
