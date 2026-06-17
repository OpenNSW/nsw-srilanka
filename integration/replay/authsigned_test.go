package replay_e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/OpenNSW/nsw-srilanka/internal/replay"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
)

// signingKid identifies the in-test signing key in the JWKS we serve.
const signingKid = "e2e-signing-key"

// memberClientID is the IdP client both MEMBER user types (Trader, CHA) sign in
// through; both carry the same nsw:* scopes, so one minter covers both.
const memberClientID = "TRADER_PORTAL_APP"

// signedAuth runs a local JWKS server and mints RS256 tokens that the REAL
// authn manager accepts. It replaces the injected-auth stub: the app runs the
// real withAuth/withScope, and tokens are validated against this server's JWKS
// — exercising the full auth-enforcement path with no IdP and no user-token gap.
//
// The pattern mirrors core/authn/test_helpers_test.go. Principals are modelled
// by kind — a MEMBER user (Trader or CHA) and a SERVICE client (an agency) —
// while flows label the per-step actor by concrete role ("trader", "fcau", …).
type signedAuth struct {
	key      *rsa.PrivateKey
	jwks     *httptest.Server
	issuer   string
	audience string
	userID   string            // the seeded MEMBER user's id (sub)
	userOU   string            // the MEMBER user's ou_handle
	tokens   map[string]string // actor -> bearer token (happy path)
}

// newSignedAuth generates a keypair, starts the JWKS server (so it is reachable
// before bootstrap.Build's authn Health() check runs), and pre-mints the
// per-actor tokens. issuer/audience must match cfg.Authn so validation passes.
func newSignedAuth(t *testing.T, issuer, audience, userID, userOU string) *signedAuth {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": signingKid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}},
		})
	}))
	t.Cleanup(jwks.Close)

	a := &signedAuth{key: key, jwks: jwks, issuer: issuer, audience: audience, userID: userID, userOU: userOU}
	// Roles are carried on the token (real IdP tokens do) to future-proof
	// role-based authz; no endpoint enforces them yet, matching current behavior.
	// A future CHA actor would mint memberClaims([]string{"CHA"}).
	a.tokens = map[string]string{
		"trader": a.sign(t, a.memberClaims([]string{"Trader"})),
		"fcau":   a.sign(t, a.serviceClaims("FCAU_TO_NSW", []string{"AgencyM2M"})),
	}
	return a
}

// baseClaims builds the claims common to every token. The real validator
// requires iss/aud/exp + a client_id in the allowlist, and routes on grant_type.
func (a *signedAuth) baseClaims(grant, clientID string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":        a.issuer,
		"aud":        a.audience,
		"client_id":  clientID,
		"grant_type": grant,
		"iat":        now.Add(-1 * time.Minute).Unix(),
		"nbf":        now.Add(-1 * time.Minute).Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
	}
}

// memberClaims mints a MEMBER (authorization_code) token for the seeded user.
// sub == the seeded idp_user_id so the middleware's GetOrCreateUser resolves
// AuthContext.User.ID back to that user. Covers both Trader and CHA (the role
// is carried on the `roles` claim for future role-based authz).
func (a *signedAuth) memberClaims(roles []string) jwt.MapClaims {
	c := a.baseClaims("authorization_code", memberClientID)
	c["sub"] = a.userID
	c["email"] = a.userID + "@example.com"
	c["ouId"] = a.userOU
	c["ouHandle"] = a.userOU
	c["roles"] = roles
	c["scope"] = strings.Join([]string{
		scopes.ConsignmentRead, scopes.ConsignmentWrite,
		scopes.TaskRead, scopes.TaskWrite,
		scopes.CompanyRead, scopes.CHARead,
		scopes.StorageRead, scopes.StorageWrite,
	}, " ")
	return c
}

// serviceClaims mints a SERVICE (client_credentials) token for an agency client.
func (a *signedAuth) serviceClaims(clientID string, roles []string) jwt.MapClaims {
	c := a.baseClaims("client_credentials", clientID)
	c["sub"] = clientID
	c["roles"] = roles
	c["scope"] = strings.Join([]string{
		scopes.TaskWrite, scopes.ConsignmentRead,
		scopes.StorageRead, scopes.StorageWrite,
	}, " ")
	return c
}

func (a *signedAuth) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = signingKid
	s, err := tok.SignedString(a.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

// transport returns a RoundTripper that swaps the engine's X-Auth-Actor header
// for a real `Authorization: Bearer <token>`. Unknown actors pass through with
// no credential (so the request is rejected by the real middleware).
func (a *signedAuth) transport() http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if actor := req.Header.Get(replay.AuthActorHeader); actor != "" {
			req = req.Clone(req.Context())
			req.Header.Del(replay.AuthActorHeader)
			if tok, ok := a.tokens[actor]; ok {
				req.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		return http.DefaultTransport.RoundTrip(req)
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
