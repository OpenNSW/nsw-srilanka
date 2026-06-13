package audit

import (
	"context"
	"encoding/json"
	"strings"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authn"
)

type contextKey string

// TraceIDKey is the context key used to propagate the Trace ID to the recorder.
const TraceIDKey contextKey = "trace_id"

// Recorder is the single entry point handlers use to emit audit events.
type Recorder struct {
	client argus.Auditor
}

// NewRecorder creates a new Recorder instance using the provided auditor client.
func NewRecorder(client argus.Auditor) *Recorder {
	return &Recorder{client: client}
}

// Event is the domain-friendly shape a handler fills in.
type Event struct {
	EventType  string
	Action     string
	TargetType string
	TargetID   string
	Failure    bool
	Message    any // Marshaled to JSON; select fields deliberately
	Metadata   map[string]any
}

// Record derives actor, trace ID, and timestamp from context, marshals the message,
// and schedules the audit log asynchronously without blocking the call path.
func (r *Recorder) Record(ctx context.Context, e Event) {
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
		EventType:  e.EventType,
		Action:     e.Action,
		Status:     status,
		ActorType:  actorType,
		ActorID:    actorID,
		TargetType: e.TargetType,
		Message:    msg,
		Metadata:   e.Metadata,
	}

	if e.TargetID != "" {
		req.TargetID = &e.TargetID
	}

	if tid := traceFrom(ctx); tid != "" {
		req.TraceID = &tid
	}

	// Detach from the request context after reading actor/trace, so client
	// disconnects do not cancel the background batch send queue.
	r.client.LogEvent(context.WithoutCancel(ctx), req)
}

func traceFrom(ctx context.Context) string {
	if v := ctx.Value(TraceIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func actorFrom(ctx context.Context) (actorType, actorID string) {
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil {
		return ActorSystem, "anonymous"
	}

	switch authCtx.Type() {
	case authn.ClientPrincipalType:
		return ActorService, authCtx.Subject()
	case authn.UserPrincipalType:
		for _, role := range authCtx.Roles() {
			if strings.EqualFold(role, "admin") {
				return ActorAdmin, authCtx.Subject()
			}
		}
		return ActorMember, authCtx.Subject()
	}

	return ActorSystem, authCtx.Subject()
}
