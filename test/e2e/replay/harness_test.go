// Package replay_e2e drives the backend end-to-end in-process by replaying an
// ordered list of API requests (defined in JSON flow files) against a real
// Postgres + Temporal started via `make deps`.
//
// Auth is REAL: the app runs the production authn middleware, and the harness
// mints RS256 tokens validated against a local in-test JWKS server (see
// signedauth_test.go) — no IdP required. Per-step identity is chosen by the
// flow's `actor` (surfaced via X-Auth-Actor and swapped for a bearer token).
//
// Actors (members and agencies) are defined in configs/members/ and
// configs/agencies/. Adding a new agency or member actor requires only a new
// JSON config file — no Go changes.
//
// Run: source .env first so DB/Temporal addresses match the running containers,
// then `make test-e2e` (or `E2E=1 go test ./test/e2e/replay/...`). Tests
// skip unless E2E=1. The `api` container must be stopped so its Temporal
// workers don't contend with the in-process workers this harness starts.
//
// See README.md for how to author new flows.
package replay_e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"

	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/internal/database"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/internal/bootstrap"
	"github.com/OpenNSW/nsw-srilanka/internal/consignment"
)

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
	// thisFile = <root>/test/e2e/replay/harness_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

// harness is an in-process app (real authn) exposed via an httptest.Server,
// plus a client that attaches minted bearer tokens per replay actor.
type harness struct {
	server  *httptest.Server
	client  *http.Client
	signed  *signedAuth
	agency  *mockAgency
	gateway *mockGateway
}

// newHarness loads config from the committed config.example.yaml, whose
// secret placeholders resolve from the environment (source .env first), seeds a
// MEMBER user, builds the app with the real authn manager pointed at a local
// JWKS server, and serves it via an httptest.Server. Torn down via t.Cleanup.
func newHarness(t *testing.T) *harness {
	t.Helper()
	root := findRepoRoot()

	members := loadMemberConfigs(t)
	agencies := loadAgencyConfigs(t)
	payments := loadPaymentConfigs(t)

	// Load reads config.yaml itself, so unlike the paths overridden below it
	// has to be redirected before the call, to the committed template.
	t.Setenv("CONFIG_PATH", filepath.Join(root, "configs", "config.example.yaml"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	// Gateways that encrypt their calls get a key pair minted for this run: the
	// app reads the private half, the mock gateway encrypts to the public half.
	paymentKeys := mintPaymentKeys(t, payments)

	// The example's payment methods point at no real key; give them the minted
	// private keys.
	setPaymentKeys(t, &cfg.Payment, payments, paymentKeys)
	cfg.Storage.Local.BaseDir = t.TempDir() // keep blob storage out of the repo tree
	// consignment_flows.json starts a flow on a consignment. The artifacts hold no other
	// consignment-level workflow, so the export workflow stands in for one.
	cfg.Consignment.Flows = []consignment.FlowOption{{TemplateID: "trade-export", Label: "Another export flow"}}

	// A controllable mock agency stands in for all external OGA services; all
	// known agency service ids are pointed at it so EXTERNAL_REVIEW injects land
	// here regardless of which agency a flow exercises. Unused by flows that don't
	// reach an external review.
	agency := newMockAgency(t, agencies)
	cfg.Services = agencyServices(agency.server.URL, agencies)

	// A controllable mock payment gateway confirms payments by posting the GovPay
	// webhook; it reads the generated reference from the payment store, so it
	// needs DB access.
	gwDB, err := database.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatalf("gateway: connect db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(gwDB) })
	// The mock reads the gateway's ids for a payment's payee from the same
	// payment methods config the app loads; it builds no gateways of its own.
	gwPayees, err := payment.NewRegistry(cfg.Payment, nil)
	if err != nil {
		t.Fatalf("gateway: load payment methods: %v", err)
	}
	gateway := newMockGateway(t, gwDB, payments, paymentKeys, gwPayees)

	// Seed one user record per member actor so each gets their own company identity.
	memberUserIDs := make(map[string]string, len(members))
	for _, m := range members {
		uid := "e2e-user-" + uuid.NewString()
		seedUser(t, cfg, uid, m.OUHandle)
		memberUserIDs[m.ID] = uid
	}

	// Run the REAL authn middleware against a JWKS server we control. Start it
	// before Build so the authn manager's Health() check (which fetches JWKS)
	// passes. Tokens are minted to match cfg.Authn (issuer/audience/client_id).
	signed := newSignedAuth(t, cfg.Authn.Issuer, cfg.Authn.Audience, memberUserIDs, members, agencies, payments)
	cfg.Authn.JWKSURL = signed.jwks.URL

	app, err := bootstrap.Build(context.Background(), cfg)
	if err != nil {
		t.Fatalf("bootstrap.Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	srv := httptest.NewServer(app.Server.Handler)
	t.Cleanup(srv.Close)

	// Now that the app is listening, wire per-agency and per-gateway bearers.
	agency.callbackBase = srv.URL
	for _, ac := range agencies {
		agency.bearers[ac.ID] = signed.tokens[ac.ID]
	}
	gateway.base = srv.URL
	for _, p := range payments {
		if tok, ok := signed.tokens[p.ID]; ok {
			gateway.bearers[p.ID] = tok
		}
	}

	return &harness{
		server:  srv,
		client:  &http.Client{Transport: signed.transport()},
		signed:  signed,
		agency:  agency,
		gateway: gateway,
	}
}

// seedUser inserts a user_records row (migrations seed companies but no users)
// whose ou_handle matches a seeded company, then deletes it on cleanup. The
// token's sub equals this id so the authn middleware resolves to this user.
func seedUser(t *testing.T, cfg *config.Config, userID, ouHandle string) {
	t.Helper()
	db, err := database.Open(context.Background(), cfg.Database)
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

// agencyServices points each agency service at its prefix on the mock server.
func agencyServices(agencyURL string, agencies []AgencyConfig) []remote.ServiceConfig {
	services := make([]remote.ServiceConfig, 0, len(agencies))
	for _, ac := range agencies {
		services = append(services, remote.ServiceConfig{ID: ac.ID, URL: agencyURL + "/" + ac.ID, Timeout: "30s"})
	}
	return services
}

// mintPaymentKeys generates a 2048-bit RSA key pair for each payment gateway
// that declares a privateKeyField, keyed by payment config id.
func mintPaymentKeys(t *testing.T, payments []PaymentConfig) map[string]*rsa.PrivateKey {
	t.Helper()
	keys := make(map[string]*rsa.PrivateKey)
	for _, p := range payments {
		if p.PrivateKeyField == "" {
			continue
		}
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("payment %q: generate rsa key: %v", p.ID, err)
		}
		keys[p.ID] = key
	}
	return keys
}

// setPaymentKeys writes each keyed gateway's minted private key to a temp PEM
// file and points the gateway's privateKeyField in cfg at it.
func setPaymentKeys(t *testing.T, cfg *payment.Config, payments []PaymentConfig, keys map[string]*rsa.PrivateKey) {
	t.Helper()
	dir := t.TempDir()
	for _, p := range payments {
		key, ok := keys[p.ID]
		if !ok {
			continue
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("payment %q: marshal private key: %v", p.ID, err)
		}
		keyPath := filepath.Join(dir, p.ID+"_private.pem")
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			t.Fatalf("payment %q: write private key: %v", p.ID, err)
		}

		method := findPaymentMethod(cfg, p.ID)
		if method == nil {
			t.Fatalf("payment %q declares privateKeyField but config.example.yaml has no payment method with that id", p.ID)
		}
		methodCfg := map[string]any{}
		if err := method.Config.Decode(&methodCfg); err != nil {
			t.Fatalf("payment %q: decode gateway config: %v", p.ID, err)
		}
		methodCfg[p.PrivateKeyField] = "file:" + keyPath
		if err := method.Config.Encode(methodCfg); err != nil {
			t.Fatalf("payment %q: encode gateway config: %v", p.ID, err)
		}
	}
}

// findPaymentMethod returns cfg's payment method with the given id, or nil.
func findPaymentMethod(cfg *payment.Config, id string) *payment.MethodConfig {
	for i := range cfg.Methods {
		if cfg.Methods[i].ID == id {
			return &cfg.Methods[i]
		}
	}
	return nil
}
