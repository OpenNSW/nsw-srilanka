package replay_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/OpenNSW/core/payment"
)

const gatewayPollInterval = 300 * time.Millisecond

// mockGateway is a controllable stand-in for the GovPay payment gateway. GovPay
// is an offline (INSTRUCTION-flow) gateway: NSW generates a TNSW reference and
// the real gateway later confirms payment via a PUBLIC, unauthenticated webhook.
// This mock simulates that webhook. It implements replay.PaymentGateway.
//
// The reference is only rendered into the task's markdown view, so the mock
// reads it from the payment store (GetByTaskID) rather than over HTTP.
type mockGateway struct {
	repo   payment.PaymentRepository
	client *http.Client
	base   string // the in-process NSW app base URL; set by the harness after start
	logf   func(string, ...any)
}

func newMockGateway(t *testing.T, db *gorm.DB) *mockGateway {
	t.Helper()
	return &mockGateway{
		repo:   payment.NewPaymentRepository(db),
		client: &http.Client{Timeout: 10 * time.Second},
		logf:   t.Logf,
	}
}

// Pay implements replay.Gateway: wait for the payment created against taskID,
// then confirm it by POSTing a GovPay success webhook. amount/currency are read
// from the payment record so they match (the handler validates them).
func (g *mockGateway) Pay(ctx context.Context, taskID, status string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(gatewayPollInterval)
	defer ticker.Stop()

	var tx *payment.PaymentTransaction
	for {
		got, err := g.repo.GetByTaskID(ctx, taskID)
		if err != nil {
			return fmt.Errorf("mock-gateway: lookup payment for task %s: %w", taskID, err)
		}
		if got != nil && got.ReferenceNumber != "" {
			tx = got
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("mock-gateway: no payment with a reference for task %s within %s", taskID, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	g.logf("mock-gateway: confirming payment ref=%s amount=%s %s (task %s)", tx.ReferenceNumber, tx.Amount.String(), tx.Currency, taskID)

	// GovPay webhook envelope (mirrors integration/payment/govpay_test.go's updateBody).
	body, err := json.Marshal(map[string]any{
		"transactionID": "e2e-gw-tx",
		"subinstId":     "e2e",
		"serviceid":     "e2e",
		"serviceName":   "Application Fee",
		"data": []map[string]any{
			{"seq": "1", "paramName": "refNo", "value": tx.ReferenceNumber},
			{"seq": "2", "paramName": "status", "value": status},
			{"seq": "3", "paramName": "amount", "value": tx.Amount.String()},
			{"seq": "4", "paramName": "currency", "value": tx.Currency},
		},
	})
	if err != nil {
		return fmt.Errorf("mock-gateway: marshal webhook: %w", err)
	}

	// Public, unauthenticated webhook endpoint.
	url := g.base + "/api/v1/payments/govpay/webhook"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("mock-gateway: webhook POST: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mock-gateway: webhook to %s got status %d: %s", url, resp.StatusCode, string(rb))
	}
	g.logf("mock-gateway: payment confirmed for task %s (status %d)", taskID, resp.StatusCode)
	return nil
}
