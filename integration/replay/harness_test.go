// Package replay_e2e drives the backend end-to-end in-process by replaying an
// ordered list of API requests (defined in JSON flow files) against a real
// Postgres + Temporal started via `make deps`.
//
// Auth is REAL: the app runs the production authn middleware, and the harness
// mints RS256 tokens validated against a local in-test JWKS server (see
// authsigned_test.go) — no IdP required. Per-step identity is chosen by the
// flow's `actor` (surfaced via X-Auth-Actor and swapped for a bearer token).
//
// Run: source .env first so DB/Temporal addresses match the running containers,
// then `make test-e2e` (or `E2E=1 go test ./integration/replay/...`). Tests
// skip unless E2E=1. The `api` container must be stopped so its Temporal
// workers don't contend with the in-process workers this harness starts.
//
// See README.md for how to author new flows.
package replay_e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"

	"github.com/OpenNSW/core/database"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/internal/bootstrap"
)

// userOUHandle is a company seeded by migrations/000003. The MEMBER user is
// seeded with this ou_handle so CreateAndStartConsignment's company lookup
// resolves. (A Trader or a CHA both resolve the same way.)
const userOUHandle = "adam-pvt-ltd"

// TestMain changes the working directory to the repo root so bootstrap.Build's
// working-dir-relative "configs" path resolves regardless of where `go test`
// runs (it otherwise uses the test package's directory). This keeps the harness
// purely additive — no production-code seam for the configs path.
func TestMain(m *testing.M) {
	if err := os.Chdir(findRepoRoot()); err != nil {
		panic("replay_e2e: chdir repo root: " + err.Error())
	}
	os.Exit(m.Run())
}

func skipUnlessE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E") != "1" {
		t.Skip("set E2E=1 to run replay E2E tests (needs `make deps` up and the api container stopped)")
	}
}

// findRepoRoot resolves the repository root from this file's location.
func findRepoRoot() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("replay_e2e: cannot determine caller location for repo root")
	}
	// thisFile = <root>/integration/replay/harness_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// harness is an in-process app (real authn) exposed via an httptest.Server,
// plus a client that attaches minted bearer tokens per replay actor.
type harness struct {
	server *httptest.Server
	client *http.Client
	signed *signedAuth
	agency *mockAgency
	userID string
}

// newHarness loads config from the environment (source .env first), seeds a
// MEMBER user, builds the app with the real authn manager pointed at a local
// JWKS server, and serves it via an httptest.Server. Torn down via t.Cleanup.
func newHarness(t *testing.T) *harness {
	t.Helper()
	root := findRepoRoot()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	// Defaults point at gitignored real config files; redirect to the committed
	// example (absolute, so it resolves irrespective of cwd).
	cfg.Server.PaymentMethodsConfigPath = filepath.Join(root, "configs", "payment_methods.example.json")
	cfg.Storage.LocalBaseDir = t.TempDir() // keep blob storage out of the repo tree

	// A controllable mock agency stands in for external OGA services; point the
	// `fcau` service at it so EXTERNAL_REVIEW injects land here. (Unused by flows
	// that don't reach an external review.)
	agency := newMockAgency(t)
	cfg.Server.ServicesConfigPath = writeServicesConfig(t, agency.server.URL)

	userID := "e2e-user-" + uuid.NewString()
	seedUser(t, cfg, userID, userOUHandle)

	// Run the REAL authn middleware against a JWKS server we control. Start it
	// before Build so the authn manager's Health() check (which fetches JWKS)
	// passes. Tokens are minted to match cfg.Authn (issuer/audience/client_id).
	signed := newSignedAuth(t, cfg.Authn.Issuer, cfg.Authn.Audience, userID, userOUHandle)
	cfg.Authn.JWKSURL = signed.jwks.URL

	app, err := bootstrap.Build(context.Background(), cfg)
	if err != nil {
		t.Fatalf("bootstrap.Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	srv := httptest.NewServer(app.Server.Handler)
	t.Cleanup(srv.Close)

	// Now that the app is listening, tell the mock agency where to call back and
	// give it a real `fcau` bearer for the authenticated callback.
	agency.callbackBase = srv.URL
	agency.bearer = signed.tokens["fcau"]

	return &harness{
		server: srv,
		client: &http.Client{Transport: signed.transport()},
		signed: signed,
		agency: agency,
		userID: userID,
	}
}

// seedUser inserts a user_records row (migrations seed companies but no users)
// whose ou_handle matches a seeded company, then deletes it on cleanup. The
// token's sub equals this id so the authn middleware resolves to this user.
func seedUser(t *testing.T, cfg *config.Config, userID, ouHandle string) {
	t.Helper()
	db, err := database.New(cfg.Database)
	if err != nil {
		t.Fatalf("seed: connect db (is `make deps` up and .env sourced?): %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM user_records WHERE id = ?", userID).Error
		_ = database.Close(db)
	})
	err = db.Exec(`INSERT INTO user_records (id, idp_user_id, email, ou_id, ou_handle, data)
		VALUES (?, ?, ?, ?, ?, '{}'::jsonb) ON CONFLICT (id) DO NOTHING`,
		userID, userID, userID+"@example.com", ouHandle, ouHandle).Error
	if err != nil {
		t.Fatalf("seed user_records: %v", err)
	}
}

// writeServicesConfig writes a temp remote-services config pointing the `fcau`
// service at the mock agency, and returns its path.
func writeServicesConfig(t *testing.T, fcauURL string) string {
	t.Helper()
	cfg := map[string]any{
		"version": "1.0",
		"services": []map[string]any{
			{"id": "fcau", "url": fcauURL, "timeout": "30s"},
		},
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal services config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write services config: %v", err)
	}
	return path
}
