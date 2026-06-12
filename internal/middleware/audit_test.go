package middleware

import (
	"bytes"
	"context"
	"crypto"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authn"
)

type mockAuditor struct {
	mu           sync.Mutex
	loggedEvents []*audit.AuditLogRequest
}

func (m *mockAuditor) LogEvent(ctx context.Context, event *audit.AuditLogRequest) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loggedEvents = append(m.loggedEvents, event)
	return true
}

func (m *mockAuditor) getEvent() *audit.AuditLogRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.loggedEvents) > 0 {
		return m.loggedEvents[0]
	}
	return nil
}

func (m *mockAuditor) SignEvent(ctx context.Context, event *audit.AuditLogRequest) error {
	return nil
}

func (m *mockAuditor) SignMessageBytes(ctx context.Context, message []byte) (string, error) {
	return "", nil
}

func (m *mockAuditor) LogSignedEvent(ctx context.Context, event *audit.AuditLogRequest) {}

func (m *mockAuditor) VerifyIntegrity(event *audit.AuditLogRequest, publicKey crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditor) IsEnabled() bool { return true }

func (m *mockAuditor) Close(ctx context.Context) error { return nil }

func TestAuditMiddleware(t *testing.T) {
	// Reset global state
	audit.ResetGlobalAuditMiddleware()

	auditor := &mockAuditor{}
	audit.InitializeGlobalAudit(auditor)

	// Create test handler
	handler := AuditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"consignment-123","status":"draft"}`))
	}))

	// Create request with auth context
	reqBody := []byte(`{"flow":"IMPORT"}`)
	req := httptest.NewRequest("POST", "/api/v1/consignments", bytes.NewBuffer(reqBody))
	req.Header.Set("X-Request-ID", "trace-789")

	authCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:    "user-456",
			Roles: []string{"trader"},
		},
	}
	ctx := context.WithValue(req.Context(), authn.AuthContextKey, authCtx)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Serve request
	handler.ServeHTTP(w, req)

	// Wait for async logging
	var event *audit.AuditLogRequest
	for i := 0; i < 50; i++ {
		if ev := auditor.getEvent(); ev != nil {
			event = ev
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if event == nil {
		t.Fatalf("expected audit event to be logged, but none was found")
	}

	if event.Action != "CREATE" {
		t.Errorf("expected Action CREATE, got %s", event.Action)
	}
	if event.ActorID != "user-456" {
		t.Errorf("expected ActorID user-456, got %s", event.ActorID)
	}
	if event.ActorType != "MEMBER" {
		t.Errorf("expected ActorType MEMBER, got %s", event.ActorType)
	}
	if event.TraceID == nil || *event.TraceID != "trace-789" {
		t.Errorf("expected TraceID trace-789, got %v", event.TraceID)
	}
	if event.TargetID == nil || *event.TargetID != "consignment-123" {
		t.Errorf("expected TargetID consignment-123, got %v", event.TargetID)
	}
	if string(event.Message) != string(reqBody) {
		t.Errorf("expected Message %s, got %s", string(reqBody), string(event.Message))
	}
}
