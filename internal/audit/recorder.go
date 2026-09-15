package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/trace"
)

// Auditor is the interface that services and handlers accept as an optional
// dependency for recording audit events.  When the value is nil, no auditing
// occurs — all implementations must be nil-receiver safe.
type Auditor interface {
	Audit(ctx context.Context, e Event)
}

// Recorder is the default Auditor implementation backed by the Argus client.
type Recorder struct {
	client argus.Auditor
}

// NewRecorder creates a new Recorder instance using the provided auditor client.
func NewRecorder(client argus.Auditor) *Recorder {
	return &Recorder{client: client}
}

// Event is the domain-friendly shape a caller fills in.
type Event struct {
	EventType  EventType
	Action     Action
	TargetType TargetType
	TargetID   string
	Failure    bool
	Message    any // Marshaled to JSON; select fields deliberately
	Metadata   map[string]any
}

// Audit derives actor, trace ID, and timestamp from context, marshals the message,
// and schedules the audit log asynchronously without blocking the call path.
// It is nil-receiver safe: a nil *Recorder is a valid no-op Auditor.
func (r *Recorder) Audit(ctx context.Context, e Event) {
	if r == nil || r.client == nil || !r.client.IsEnabled() {
		return
	}

	actorType, actorID := actorFrom(ctx)

	status := argus.StatusSuccess
	if e.Failure {
		status = argus.StatusFailure
	}

	var msg []byte
	if e.Message != nil {
		if raw, ok := e.Message.([]byte); ok {
			msg = raw
		} else {
			msg, _ = json.Marshal(e.Message)
		}
	}

	req := &argus.AuditLogRequest{
		Timestamp:  argus.CurrentTimestamp(),
		EventType:  string(e.EventType),
		Action:     string(e.Action),
		Status:     status,
		ActorType:  string(actorType),
		ActorID:    actorID,
		TargetType: string(e.TargetType),
		Message:    msg,
		Metadata:   e.Metadata,
	}

	if e.TargetID != "" {
		req.TargetID = &e.TargetID
	}

	if tid := trace.GetTraceID(ctx); tid != "" {
		req.TraceID = &tid
	}

	// Detach from the request context after reading actor/trace, so client
	// disconnects do not cancel the background batch send queue.
	r.client.LogEvent(context.WithoutCancel(ctx), req)
}

func actorFrom(ctx context.Context) (ActorType, string) {
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil {
		return ActorSystem, "anonymous"
	}

	switch authCtx.Type() {
	case authn.ClientPrincipalType:
		return ActorService, authCtx.Subject()
	case authn.UserPrincipalType:
		// Treated as ActorMember for now as no Admin role is defined in this phase.
		return ActorMember, authCtx.Subject()
	default:
		slog.ErrorContext(ctx, "audit: unrecognized principal type encountered", "type", authCtx.Type())
		return ActorSystem, authCtx.Subject()
	}
}
