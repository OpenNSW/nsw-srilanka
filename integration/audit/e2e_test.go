// e2e_test.go — End-to-end integration test: API → AuditMiddleware → Argus → PostgreSQL.
//
// This test verifies the complete audit logging pipeline by:
//
//  1. Injecting an authenticated user context (simulating what withAuth does)
//  2. Exercising the real AuditMiddleware + a handler that returns 201
//  3. Waiting for the Argus async client to flush
//  4. Querying the Argus REST API to verify the audit entry was persisted
//
// Prerequisites (docker compose up):
//   - Argus on localhost:3001 (docker: nsw-argus)
//
// Run:
//
//	E2E=1 go test -v -count=1 ./integration/audit/...
package audit_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/middleware"
)

func skipUnlessE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E") != "1" {
		t.Skip("Skipping E2E test: set E2E=1 to run")
	}
}

// argusAuditLog represents a single audit log entry from the Argus REST API.
type argusAuditLog struct {
	ID         string `json:"id"`
	EventType  string `json:"eventType"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	ActorType  string `json:"actorType"`
	ActorID    string `json:"actorId"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	CreatedAt  string `json:"createdAt"`
}

// argusListResponse is the Argus API response for GET /api/audit-logs
type argusListResponse struct {
	Logs []argusAuditLog `json:"logs"`
}

// TestConsignmentCreateAuditE2E tests that a POST request through the
// AuditMiddleware produces an audit log entry in Argus's database.
func TestConsignmentCreateAuditE2E(t *testing.T) {
	skipUnlessE2E(t)

	argusURL := envOrDefault("ARGUS_SERVICE_URL", "http://localhost:3001")
	argusToken := envOrDefault("ARGUS_AUTH_TOKEN", "nsw-super-secret-audit-token")

	// ---------------------------------------------------------------
	// 1. Initialize the real Argus audit client
	// ---------------------------------------------------------------
	auditClient := audit.NewClient(audit.Config{
		BaseURL:   argusURL,
		AuthToken: argusToken,
	})
	if !auditClient.IsEnabled() {
		t.Fatal("Audit client is not enabled — is Argus running on " + argusURL + "?")
	}
	audit.InitializeGlobalAudit(auditClient)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = auditClient.Close(ctx)
	})

	// ---------------------------------------------------------------
	// 2. Record the baseline count from the Argus REST API
	// ---------------------------------------------------------------
	baselineLogs := listAuditLogs(t, argusURL, argusToken)
	baselineCount := len(baselineLogs)
	t.Logf("Baseline audit log count: %d", baselineCount)

	// ---------------------------------------------------------------
	// 3. Create a mock handler behind AuditMiddleware that returns 201
	// ---------------------------------------------------------------
	consignmentID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":     consignmentID,
			"status": "DRAFT",
			"flow":   "EXPORT",
		})
	})

	auditHandler := middleware.AuditMiddleware(innerHandler)

	// ---------------------------------------------------------------
	// 4. Build a request with an injected AuthContext
	// ---------------------------------------------------------------
	req := httptest.NewRequest("POST", "/api/v1/consignments", nil)
	req.Header.Set("Content-Type", "application/json")

	authCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:       "e2e-test-user-id",
			Email:    "e2e-test@example.com",
			OUHandle: "test-ou",
			Roles:    []string{"Trader"},
			Scopes:   []string{"nsw:consignment:write"},
		},
	}
	ctx := context.WithValue(req.Context(), authn.AuthContextKey, authCtx)
	req = req.WithContext(ctx)

	// ---------------------------------------------------------------
	// 5. Execute the request
	// ---------------------------------------------------------------
	rr := httptest.NewRecorder()
	auditHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Logf("Handler responded: %d — %s", rr.Code, rr.Body.String())

	// ---------------------------------------------------------------
	// 6. Wait for the async audit flush (batch interval = 1s + network)
	// ---------------------------------------------------------------
	t.Log("Waiting 4s for audit batch flush to Argus...")
	time.Sleep(4 * time.Second)

	// ---------------------------------------------------------------
	// 7. Query Argus REST API for the new audit logs
	// ---------------------------------------------------------------
	afterLogs := listAuditLogs(t, argusURL, argusToken)
	afterCount := len(afterLogs)
	t.Logf("Audit log count after request: %d (was: %d)", afterCount, baselineCount)

	if afterCount <= baselineCount {
		t.Fatalf("FAIL: No new audit log was written! Expected count > %d, got %d", baselineCount, afterCount)
	}

	// ---------------------------------------------------------------
	// 8. Find and verify our specific audit entry
	// ---------------------------------------------------------------
	var found *argusAuditLog
	for i := range afterLogs {
		if afterLogs[i].ActorID == "e2e-test-user-id" && afterLogs[i].TargetID == consignmentID {
			found = &afterLogs[i]
			break
		}
	}

	if found == nil {
		// Fallback: check the latest entry
		latest := afterLogs[0] // Argus returns newest first
		t.Logf("Could not find exact match; latest entry: %+v", latest)

		if latest.ActorID == "e2e-test-user-id" {
			found = &latest
		} else {
			t.Fatalf("FAIL: Could not find the audit entry for actor=e2e-test-user-id target=%s", consignmentID)
		}
	}

	t.Logf("Found audit entry: id=%s action=%s actor=%s target=%s status=%s",
		found.ID, found.Action, found.ActorID, found.TargetID, found.Status)

	if found.Action != "CREATE" {
		t.Errorf("expected action=CREATE, got %s", found.Action)
	}
	if found.EventType != "MANAGEMENT_EVENT" {
		t.Errorf("expected eventType=MANAGEMENT_EVENT, got %s", found.EventType)
	}
	if found.ActorType != "MEMBER" {
		t.Errorf("expected actorType=MEMBER, got %s", found.ActorType)
	}
	if found.Status != "SUCCESS" {
		t.Errorf("expected status=SUCCESS, got %s", found.Status)
	}

	t.Log("✅ E2E audit test passed: POST /api/v1/consignments → AuditMiddleware → Argus → DB")
}

// listAuditLogs fetches all audit logs from the Argus REST API.
func listAuditLogs(t *testing.T, argusURL, token string) []argusAuditLog {
	t.Helper()

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", argusURL+"/api/audit-logs", nil)
	if err != nil {
		t.Fatalf("failed to create Argus request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to query Argus: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Argus returned %d: %s", resp.StatusCode, string(body))
	}

	var result argusListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode Argus response: %v", err)
	}

	return result.Logs
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
