package storage

import (
	"context"

	corestorage "github.com/OpenNSW/core/storage"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// Adapter bridges the core storage.Auditor interface to the nsw-srilanka
// nswaudit.Auditor used across the application.
type Adapter struct {
	auditor nswaudit.Auditor
}

// NewAuditAdapter creates a storage.Auditor backed by the application auditor.
func NewAuditAdapter(a nswaudit.Auditor) *Adapter {
	return &Adapter{auditor: a}
}

func (a *Adapter) AuditStorage(ctx context.Context, e corestorage.AuditEvent) {
	if a.auditor == nil {
		return
	}
	a.auditor.Audit(ctx, nswaudit.Event{
		EventType:  nswaudit.EventStorage,
		Action:     nswaudit.Action(e.Action),
		TargetType: nswaudit.TargetStorage,
		TargetID:   e.Key,
		Failure:    e.Failure,
		Metadata: map[string]any{
			"filename": e.Filename,
			"mimeType": e.MimeType,
			"size":     e.Size,
		},
	})
}
