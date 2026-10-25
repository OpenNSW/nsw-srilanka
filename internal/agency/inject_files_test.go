package agency

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
)

// injectingClient is the machine client TNSW injects as, which this agency's
// catalog says is the service tnsw.
var injectingClient = &authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_CDA"}

func testAccess(t *testing.T) *fileaccess.Access {
	t.Helper()
	keys, err := filetoken.ParseKeyset("k1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)))
	require.NoError(t, err)
	codec, err := filetoken.NewCodec(keys, nil)
	require.NoError(t, err)
	access, err := fileaccess.New(codec, time.Hour, fileaccess.WithClientServices(func(id string) (string, bool) {
		return "tnsw", id == "NSW_TO_CDA"
	}))
	require.NoError(t, err)
	return access
}

func registryWithFiles(files string) *artifact.Registry {
	reg := artifact.NewRegistry(testutil.MemLoader{
		"verify.json":    []byte(`{"schemaVersion":2,"taskCode":"verify","workflow":"verify_wf",` + files + `"meta":{"title":"Verify"}}`),
		"verify_wf.json": []byte(`{"id":"verify_wf","nodes":[],"edges":[]}`),
	})
	reg.RegisterArtifact("verify", "task_config", "", "verify.json")
	reg.RegisterArtifact("verify_wf", "workflow", "", "verify_wf.json")
	return reg
}

func inject(t *testing.T, svc *Service, p *authn.Principal, data string) (*Workflow, error) {
	t.Helper()
	ctx := context.Background()
	if p != nil {
		ctx = authn.ContextWithPrincipal(ctx, p)
	}
	return svc.Inject(ctx, InjectRequest{TaskID: "T1", TaskCode: "verify", ConsignmentID: "C1", Data: json.RawMessage(data)})
}

// The files a task config declares are stored as the sending service's; nothing
// else in the payload changes.
func TestInject_TagsDeclaredFiles(t *testing.T) {
	repo := &startingRepo{}
	svc := NewService(repo, registryWithFiles(`"files":["invoice","documents[*].file"],`), &startCapture{}, WithFiles(testAccess(t)))

	data := `{"invoice":"tok-1","documents":[{"file":"tok-2","name":"a<b>"},{"file":"tok-3"}],"note":"tok-4","amount":12345678901234567890,"empty":""}`
	_, err := inject(t, svc, injectingClient, data)
	require.NoError(t, err)

	var got map[string]any
	dec := json.NewDecoder(bytes.NewReader(repo.recorded.Payload))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&got))
	assert.Equal(t, "tnsw:tok-1", got["invoice"])
	docs := got["documents"].([]any)
	assert.Equal(t, "tnsw:tok-2", docs[0].(map[string]any)["file"])
	assert.Equal(t, "a<b>", docs[0].(map[string]any)["name"])
	assert.Equal(t, "tnsw:tok-3", docs[1].(map[string]any)["file"])
	assert.Equal(t, "tok-4", got["note"], "an undeclared path is left alone")
	assert.Equal(t, json.Number("12345678901234567890"), got["amount"], "numbers stay exact")
	assert.Contains(t, string(repo.recorded.Payload), `"a<b>"`, "text is not HTML-escaped")
}

func TestInject_WithoutDeclaredFilesStoresThePayloadAsSent(t *testing.T) {
	repo := &startingRepo{}
	svc := NewService(repo, registryWithFiles(``), &startCapture{})

	const data = `{ "invoice" : "tok-1",  "n": 1.50 }`
	_, err := inject(t, svc, nil, data)
	require.NoError(t, err)
	assert.JSONEq(t, data, string(repo.recorded.Payload))
	assert.Equal(t, data, string(repo.recorded.Payload))
}

func TestInject_RefusesWhatTheCallerMayNotSend(t *testing.T) {
	user := &authn.Principal{Kind: authn.KindUser, IDPUserID: "u1"}
	unlisted := &authn.Principal{Kind: authn.KindClient, ClientID: "SLPA_TO_NSW"}
	for name, p := range map[string]*authn.Principal{"a user": user, "a client that is no service": unlisted, "no caller": nil} {
		t.Run(name, func(t *testing.T) {
			svc := NewService(recordingRepo{t: t}, registryWithFiles(`"files":["invoice"],`), nil, WithFiles(testAccess(t)))
			_, err := inject(t, svc, p, `{"invoice":"tok-1"}`)
			assert.ErrorIs(t, err, ErrInvalidFiles)
		})
	}
}

func TestInject_RefusesAShapeThatIsNotDeclared(t *testing.T) {
	for name, data := range map[string]string{
		"a list where one file is declared": `{"invoice":["tok-1"]}`,
		"an object":                         `{"invoice":{"a":1}}`,
		"a number":                          `{"invoice":7}`,
		"a payload that is not an object":   `["x"]`,
		"a payload that is not JSON":        `{`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := NewService(recordingRepo{t: t}, registryWithFiles(`"files":["invoice"],`), nil, WithFiles(testAccess(t)))
			_, err := inject(t, svc, injectingClient, data)
			assert.ErrorIs(t, err, ErrInvalidFiles)
		})
	}
}

// A task config that declares files cannot be injected without anything to accept
// them.
func TestInject_DeclaredFilesNeedAnAcceptor(t *testing.T) {
	svc := NewService(recordingRepo{t: t}, registryWithFiles(`"files":["invoice"],`), nil)
	_, err := inject(t, svc, injectingClient, `{"invoice":"tok-1"}`)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrInvalidFiles))
}

// A retried inject keeps the first payload, with its files as first stored.
func TestInject_RetryKeepsTheFirstPayload(t *testing.T) {
	repo := &startingRepo{}
	svc := NewService(repo, registryWithFiles(`"files":["invoice"],`), &startCapture{}, WithFiles(testAccess(t)))
	_, err := inject(t, svc, injectingClient, `{"invoice":"first"}`)
	require.NoError(t, err)
	_, err = inject(t, svc, injectingClient, `{"invoice":"second"}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"invoice":"tnsw:first"}`, string(repo.recorded.Payload))
}

func handleInject(svc *Service, p *authn.Principal, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inject", strings.NewReader(body))
	if p != nil {
		req = req.WithContext(authn.ContextWithPrincipal(req.Context(), p))
	}
	rec := httptest.NewRecorder()
	svc.HandleInject(rec, req)
	return rec
}

func TestHandleInject_DoesNotEchoThePayloadAndMapsFileErrors(t *testing.T) {
	svc := NewService(&startingRepo{}, registryWithFiles(`"files":["invoice"],`), &startCapture{}, WithFiles(testAccess(t)))

	rec := handleInject(svc, injectingClient, `{"taskId":"T1","taskCode":"verify","consignmentId":"C1","data":{"invoice":"tok-1"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "tok-1")
	assert.NotContains(t, rec.Body.String(), "payload")

	rec = handleInject(svc, &authn.Principal{Kind: authn.KindUser, IDPUserID: "u1"}, `{"taskId":"T2","taskCode":"verify","consignmentId":"C1","data":{"invoice":"tok-1"}}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.NotContains(t, rec.Body.String(), "tok-1")
}

func TestHandleInject_LimitsTheBody(t *testing.T) {
	svc := NewService(&startingRepo{}, registryWithFiles(``), &startCapture{}, WithMaxBodyBytes(64))
	rec := handleInject(svc, injectingClient, `{"taskId":"T1","taskCode":"verify","consignmentId":"C1","data":{"x":"`+strings.Repeat("a", 200)+`"}}`)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
