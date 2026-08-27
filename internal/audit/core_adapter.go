package audit

import (
	"context"

	sharedaudit "github.com/OpenNSW/core/shared/audit"
)

// CoreAdapter bridges core/shared/audit.Auditor (used by core/payment and
// core/storage) to the application Auditor that writes through Argus.
type CoreAdapter struct {
	auditor Auditor
}

// NewCoreAdapter wraps an application Auditor as a shared/audit.Auditor.
func NewCoreAdapter(a Auditor) *CoreAdapter {
	return &CoreAdapter{auditor: a}
}

// Audit implements sharedaudit.Auditor.
func (a *CoreAdapter) Audit(ctx context.Context, e sharedaudit.Event) {
	if a == nil || a.auditor == nil {
		return
	}

	var (
		eventType  EventType
		targetType TargetType
		targetID   string
		metadata   map[string]any
	)

	switch e.Domain {
	case sharedaudit.DomainPayment:
		eventType = EventPayment
		targetType = TargetPayment
		targetID = e.Reference
		metadata = map[string]any{
			"gatewayId": e.GatewayID,
			"status":    e.Status,
		}
	case sharedaudit.DomainStorage:
		eventType = EventStorage
		targetType = TargetStorage
		targetID = e.Key
		metadata = map[string]any{
			"filename": e.Filename,
			"mimeType": e.MimeType,
			"size":     e.Size,
		}
	default:
		return
	}

	if e.Error != "" {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["error"] = e.Error
	}

	a.auditor.Audit(ctx, Event{
		EventType:  eventType,
		Action:     Action(e.Action),
		TargetType: targetType,
		TargetID:   targetID,
		Failure:    e.Failure,
		Metadata:   metadata,
	})
}
