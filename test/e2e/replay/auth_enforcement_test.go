package replay_e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
)

// TestReplay_AuthEnforcement verifies the REAL withAuth/withScope reject bad
// tokens. Positive coverage comes from the flow tests, which run with valid
// minted tokens; here we assert the negative paths. All subtests share one
// harness (one app build).
func TestReplay_AuthEnforcement(t *testing.T) {
	skipUnlessE2E(t)
	h := newHarness(t)
	plain := &http.Client{} // no per-actor bearer injection — we set Authorization ourselves

	// postConsignment sends POST /api/v1/consignments with an optional bearer
	// token and returns the status code.
	postConsignment := func(t *testing.T, bearer string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, h.server.URL+"/api/v1/consignments", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := plain.Do(req)
		if err != nil {
			t.Fatalf("POST consignments: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	t.Run("no token -> 401", func(t *testing.T) {
		if got := postConsignment(t, ""); got != http.StatusUnauthorized {
			t.Errorf("no token: got %d, want 401", got)
		}
	})

	t.Run("missing required scope -> 403", func(t *testing.T) {
		// A valid member token that lacks nsw:consignment:write.
		c := h.signed.baseClaims("authorization_code", memberClientID)
		c["sub"] = h.userID
		c["email"] = h.userID + "@example.com"
		c["ouId"] = userOUHandle
		c["ouHandle"] = userOUHandle
		c["roles"] = []string{"Trader"}
		c["scope"] = scopes.ConsignmentRead // read only
		if got := postConsignment(t, h.signed.sign(t, c)); got != http.StatusForbidden {
			t.Errorf("missing scope: got %d, want 403", got)
		}
	})

	t.Run("expired token -> 401", func(t *testing.T) {
		c := h.signed.memberClaims([]string{"Trader"})
		c["exp"] = time.Now().Add(-1 * time.Hour).Unix()
		if got := postConsignment(t, h.signed.sign(t, c)); got != http.StatusUnauthorized {
			t.Errorf("expired token: got %d, want 401", got)
		}
	})
}
