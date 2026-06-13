package audit

import (
	"context"
	"crypto"
	"sync"
	"testing"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authn"
)

type mockAuditor struct {
	mu           sync.Mutex
	loggedEvents []*argus.AuditLogRequest
	enabled      bool
}

func (m *mockAuditor) LogEvent(ctx context.Context, event *argus.AuditLogRequest) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled {
		return false
	}
	m.loggedEvents = append(m.loggedEvents, event)
	return true
}

func (m *mockAuditor) getEvents() []*argus.AuditLogRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loggedEvents
}

func (m *mockAuditor) SignEvent(ctx context.Context, event *argus.AuditLogRequest) error {
	return nil
}

func (m *mockAuditor) SignMessageBytes(ctx context.Context, message []byte) (string, error) {
	return "", nil
}

func (m *mockAuditor) LogSignedEvent(ctx context.Context, event *argus.AuditLogRequest) {}

func (m *mockAuditor) VerifyIntegrity(event *argus.AuditLogRequest, publicKey crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditor) IsEnabled() bool {
	return m.enabled
}

func (m *mockAuditor) Close(ctx context.Context) error {
	return nil
}

func TestRecorder_Record(t *testing.T) {
	client := &mockAuditor{enabled: true}
	recorder := NewRecorder(client)

	// 1. Test Actor Type derivation (MEMBER / Admin)
	userCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:    "user-1",
			Roles: []string{"trader"},
		},
	}
	ctx := context.WithValue(context.Background(), authn.AuthContextKey, userCtx)
	ctx = context.WithValue(ctx, TraceIDKey, "trace-1")

	recorder.Record(ctx, Event{
		EventType:  EventConsignment,
		Action:     ActionCreate,
		TargetType: TargetConsignment,
		TargetID:   "con-123",
		Failure:    false,
		Message:    map[string]string{"foo": "bar"},
	})

	events := client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}

	ev := events[0]
	if ev.ActorType != ActorMember {
		t.Errorf("expected ActorType %s, got %s", ActorMember, ev.ActorType)
	}
	if ev.ActorID != "user-1" {
		t.Errorf("expected ActorID user-1, got %s", ev.ActorID)
	}
	if ev.TraceID == nil || *ev.TraceID != "trace-1" {
		t.Errorf("expected TraceID trace-1, got %v", ev.TraceID)
	}
	if ev.Status != argus.StatusSuccess {
		t.Errorf("expected Status SUCCESS, got %s", ev.Status)
	}

	// 2. Test Admin actor type derivation
	client.mu.Lock()
	client.loggedEvents = nil
	client.mu.Unlock()

	adminCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:    "admin-1",
			Roles: []string{"admin"},
		},
	}
	ctx = context.WithValue(context.Background(), authn.AuthContextKey, adminCtx)
	recorder.Record(ctx, Event{
		EventType:  EventConsignment,
		Action:     ActionCreate,
		TargetType: TargetConsignment,
		TargetID:   "con-123",
		Failure:    true,
	})

	events = client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}
	ev = events[0]
	if ev.ActorType != ActorAdmin {
		t.Errorf("expected ActorType %s, got %s", ActorAdmin, ev.ActorType)
	}
	if ev.Status != argus.StatusFailure {
		t.Errorf("expected Status FAILURE, got %s", ev.Status)
	}

	// 3. Test Service client actor type derivation
	client.mu.Lock()
	client.loggedEvents = nil
	client.mu.Unlock()

	clientCtx := &authn.AuthContext{
		Client: &authn.ClientContext{
			ClientID: "service-1",
		},
	}
	ctx = context.WithValue(context.Background(), authn.AuthContextKey, clientCtx)
	recorder.Record(ctx, Event{
		EventType:  EventTask,
		Action:     ActionUpdate,
		TargetType: TargetTask,
	})

	events = client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}
	ev = events[0]
	if ev.ActorType != ActorService {
		t.Errorf("expected ActorType %s, got %s", ActorService, ev.ActorType)
	}
	if ev.ActorID != "service-1" {
		t.Errorf("expected ActorID service-1, got %s", ev.ActorID)
	}

	// 4. Test System / Anonymous fallback
	client.mu.Lock()
	client.loggedEvents = nil
	client.mu.Unlock()

	recorder.Record(context.Background(), Event{
		EventType:  EventStorage,
		Action:     ActionDelete,
		TargetType: TargetStorage,
	})

	events = client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}
	ev = events[0]
	if ev.ActorType != ActorSystem {
		t.Errorf("expected ActorType %s, got %s", ActorSystem, ev.ActorType)
	}
	if ev.ActorID != "anonymous" {
		t.Errorf("expected ActorID anonymous, got %s", ev.ActorID)
	}
}

func TestRecorder_Disabled(t *testing.T) {
	client := &mockAuditor{enabled: false}
	recorder := NewRecorder(client)

	recorder.Record(context.Background(), Event{
		EventType:  EventConsignment,
		Action:     ActionCreate,
		TargetType: TargetConsignment,
	})

	events := client.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events because auditor client is disabled, got %d", len(events))
	}
}
