package audit_test

import (
	"context"
	"testing"

	sharedaudit "github.com/OpenNSW/core/shared/audit"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureAuditor struct {
	events []nswaudit.Event
}

func (c *captureAuditor) Audit(_ context.Context, e nswaudit.Event) {
	c.events = append(c.events, e)
}

func TestCoreAdapter_Payment(t *testing.T) {
	cap := &captureAuditor{}
	adapter := nswaudit.NewCoreAdapter(cap)

	adapter.Audit(context.Background(), sharedaudit.Event{
		Domain:    sharedaudit.DomainPayment,
		Action:    sharedaudit.ActionCheckout,
		GatewayID: "govpay",
		Reference: "REF-1",
		Status:    "PENDING",
	})

	require.Len(t, cap.events, 1)
	got := cap.events[0]
	assert.Equal(t, nswaudit.EventPayment, got.EventType)
	assert.Equal(t, nswaudit.ActionCreate, got.Action)
	assert.Equal(t, nswaudit.TargetPayment, got.TargetType)
	assert.Equal(t, "REF-1", got.TargetID)
	assert.Equal(t, "govpay", got.Metadata["gatewayId"])
	assert.Equal(t, "PENDING", got.Metadata["status"])
	assert.False(t, got.Failure)
}

func TestCoreAdapter_StorageFailure(t *testing.T) {
	cap := &captureAuditor{}
	adapter := nswaudit.NewCoreAdapter(cap)

	adapter.Audit(context.Background(), sharedaudit.Event{
		Domain:   sharedaudit.DomainStorage,
		Action:   sharedaudit.ActionPresignUpload,
		Key:      "obj-1",
		Filename: "doc.pdf",
		MimeType: "application/pdf",
		Size:     12,
		Failure:  true,
		Error:    "boom",
	})

	require.Len(t, cap.events, 1)
	got := cap.events[0]
	assert.Equal(t, nswaudit.EventStorage, got.EventType)
	assert.Equal(t, nswaudit.ActionPresignUpload, got.Action)
	assert.Equal(t, nswaudit.TargetStorage, got.TargetType)
	assert.Equal(t, "obj-1", got.TargetID)
	assert.True(t, got.Failure)
	assert.Equal(t, "boom", got.Metadata["error"])
	assert.Equal(t, "doc.pdf", got.Metadata["filename"])
}

func TestCoreAdapter_NilSafe(t *testing.T) {
	assert.NotPanics(t, func() {
		var adapter *nswaudit.CoreAdapter
		adapter.Audit(context.Background(), sharedaudit.Event{Domain: sharedaudit.DomainPayment})
		nswaudit.NewCoreAdapter(nil).Audit(context.Background(), sharedaudit.Event{Domain: sharedaudit.DomainPayment})
	})
}
