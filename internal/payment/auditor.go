package payment

import (
	"context"

	corepayment "github.com/OpenNSW/core/payment"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// Adapter bridges the core payment.Auditor interface to the nsw-srilanka
// nswaudit.Auditor used across the application.
type Adapter struct {
	auditor nswaudit.Auditor
}

// NewAuditAdapter creates a payment.Auditor backed by the application auditor.
func NewAuditAdapter(a nswaudit.Auditor) *Adapter {
	return &Adapter{auditor: a}
}

func (a *Adapter) AuditPayment(ctx context.Context, e corepayment.AuditEvent) {
	if a.auditor == nil {
		return
	}
	a.auditor.Audit(ctx, nswaudit.Event{
		EventType:  nswaudit.EventPayment,
		Action:     nswaudit.Action(e.Action),
		TargetType: nswaudit.TargetPayment,
		TargetID:   e.Reference,
		Failure:    e.Failure,
		Metadata: map[string]any{
			"gatewayId": e.GatewayID,
			"status":    e.Status,
		},
	})
}
