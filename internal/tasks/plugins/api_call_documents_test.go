package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/gatepass"
	"github.com/OpenNSW/nsw-srilanka/internal/documents"
)

type memStore struct{ names []string }

func (s *memStore) Store(_ context.Context, filename string, _ []byte, _ string) (string, error) {
	s.names = append(s.names, filename)
	return fmt.Sprintf("00000000-0000-0000-0000-%012d%s", len(s.names), filepath.Ext(filename)), nil
}

// fakeCMS answers the gate-pass call and serves the pass it links to, on the
// same host — the way SLPA's CMS does.
func fakeCMS(t *testing.T, refuse bool) (*remote.Manager, string) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/orders/s1/generate-gatepass":
			w.Header().Set("Content-Type", "application/json")
			if refuse {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"status":0,"error":{"message":"invalid container"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 1, "data": map[string]any{
				"gate_pass_no":  "756895",
				"barcode":       "data:image/png;base64,iVBORw0KGgo=",
				"gate_pass_url": srv.URL + "/pdf/gate-pass/756895?signature=abc",
			}})
		case "/pdf/gate-pass/756895":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "services.json")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`{"services":[{"id":"slpa","url":%q,"timeout":"5s"}]}`, srv.URL)), 0o600))
	mgr := remote.NewManager()
	require.NoError(t, mgr.LoadServices(path))
	return mgr, srv.URL
}

func runGatePass(t *testing.T, mgr *remote.Manager, archiver *documents.Archiver) map[string]any {
	t.Helper()
	plugin := NewAPICallPluginWithInterpreter(mgr, gatepass.NewInterpreter()).WithDocuments(archiver)
	ctx := pluginContext{
		Context: context.Background(),
		Record:  &store.TaskRecord{TaskID: "task-1", Data: map[string]any{}},
		Inputs: map[string]any{
			"slug": "s1", "so_container_no": "DUMY0000001",
			"payload": map[string]any{"truck_no": "WP-1", "driver_name": "D", "seal_no": "S"},
		},
		OutputNamespace: "gp",
	}
	cfg := `{"service_id":"slpa","path":"/orders/{slug}/generate-gatepass","result_field":"issued"}`
	require.NoError(t, plugin.Execute(ctx, json.RawMessage(cfg)))
	gp, _ := ctx.Record.Data["gp"].(map[string]any)
	return gp
}

func TestAPICallKeepsTheDocumentsAnAcceptedAnswerLinksTo(t *testing.T) {
	mgr, _ := fakeCMS(t, false)
	store := &memStore{}

	gp := runGatePass(t, mgr, documents.NewArchiver(store, mgr))

	assert.Equal(t, true, gp["issued"])
	assert.Equal(t, "756895", gp["gate_pass_no"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000001.pdf", gp["gate_pass_url"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000002.png", gp["barcode"])
	assert.Equal(t, []string{"slpa-gate-pass.pdf", "slpa-gate-pass-barcode.png"}, store.names)
}

func TestAPICallWithoutAnArchiverKeepsTheProvidersLinks(t *testing.T) {
	mgr, base := fakeCMS(t, false)

	gp := runGatePass(t, mgr, nil)

	assert.Equal(t, base+"/pdf/gate-pass/756895?signature=abc", gp["gate_pass_url"])
	assert.Equal(t, "data:image/png;base64,iVBORw0KGgo=", gp["barcode"])
}

func TestAPICallArchivesNothingForARefusedAnswer(t *testing.T) {
	mgr, _ := fakeCMS(t, true)
	store := &memStore{}

	gp := runGatePass(t, mgr, documents.NewArchiver(store, mgr))

	assert.Equal(t, false, gp["issued"])
	assert.Empty(t, store.names)
}
