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
)

// signingKid identifies the in-test signing key in the JWKS we serve.
const signingKid = "e2e-signing-key"

// signedAuth runs a local JWKS server and mints RS256 tokens that the REAL
// authn manager accepts. It replaces the injected-auth stub: the app runs the
// real withAuth/withScope, and tokens are validated against this server's JWKS
// — exercising the full auth-enforcement path with no IdP and no user-token gap.
//
// Principals are driven entirely from configs/members/ and configs/agencies/.
// Adding a new actor requires only a new JSON config file, no code change here.
type signedAuth struct {
	key      *rsa.PrivateKey
	jwks     *httptest.Server
	issuer   string
	audience string
	tokens   map[string]string // actor id -> bearer token
}

// newSignedAuth generates a keypair, starts the JWKS server (reachable before
// bootstrap.Build's authn Health() check), and pre-mints tokens for every member,
// agency, and payment gateway config that carries an identity. issuer/audience must
// match cfg.Authn so validation passes. memberUserIDs maps each member ID to its
// seeded DB user id (sub claim).
func newSignedAuth(t *testing.T, issuer, audience string, memberUserIDs map[string]string, members []MemberConfig, agencies []AgencyConfig, payments []PaymentConfig) *signedAuth {
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

	a := &signedAuth{
		key:      key,
		jwks:     jwks,
		issuer:   issuer,
		audience: audience,
		tokens:   make(map[string]string),
	}
	for _, m := range members {
		a.tokens[m.ID] = a.sign(t, a.memberClaims(m, memberUserIDs[m.ID]))
	}
	for _, ag := range agencies {
		a.tokens[ag.ID] = a.sign(t, a.serviceClaims(ag.Identity))
	}
	for _, p := range payments {
		if p.Identity != nil {
			a.tokens[p.ID] = a.sign(t, a.serviceClaims(*p.Identity))
		}
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

// memberClaims mints an authorization_code token for a specific seeded user.
// sub == the seeded idp_user_id so the middleware's GetOrCreateUser resolves
// AuthContext.User.ID back to that user.
func (a *signedAuth) memberClaims(m MemberConfig, userID string) jwt.MapClaims {
	c := a.baseClaims("authorization_code", m.Identity.ClientID)
	c["sub"] = userID
	c["email"] = userID + "@example.com"
	c["ouId"] = m.OUHandle
	c["ouHandle"] = m.OUHandle
	c["roles"] = m.Identity.Roles
	c["scope"] = strings.Join(m.Identity.Scopes, " ")
	return c
}

// serviceClaims mints a client_credentials token for a SERVICE (M2M) agency client.
func (a *signedAuth) serviceClaims(id ActorIdentity) jwt.MapClaims {
	c := a.baseClaims("client_credentials", id.ClientID)
	c["sub"] = id.ClientID
	c["roles"] = id.Roles
	c["scope"] = strings.Join(id.Scopes, " ")
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
