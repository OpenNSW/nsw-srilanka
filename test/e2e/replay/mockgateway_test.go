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

// mockGateway is a controllable stand-in for an offline (INSTRUCTION-flow)
// payment gateway: NSW generates a TNSW reference and the real gateway later
// confirms payment via a webhook protected by an M2M bearer token (see
// PaymentConfig.Identity / signedAuth.tokens). This mock simulates that
// webhook, driven entirely by configs/payments/<id>.json — it carries no
// knowledge of any specific gateway. It implements replay.PaymentGateway.
//
// The reference is only rendered into the task's markdown view, so the mock
// reads it from the payment store (GetByTaskID) rather than over HTTP.
type mockGateway struct {
	repo    payment.PaymentRepository
	client  *http.Client
	base    string // the in-process NSW app base URL; set by the harness after start
	configs map[string]PaymentConfig
	bearers map[string]string // paymentID -> SERVICE bearer token (empty = unauthenticated)
	logf    func(string, ...any)
}

func newMockGateway(t *testing.T, db *gorm.DB, configs []PaymentConfig) *mockGateway {
	t.Helper()
	cfgMap := make(map[string]PaymentConfig, len(configs))
	for _, c := range configs {
		cfgMap[c.ID] = c
	}
	return &mockGateway{
		repo:    payment.NewPaymentRepository(db),
		client:  &http.Client{Timeout: 10 * time.Second},
		configs: cfgMap,
		bearers: make(map[string]string),
		logf:    t.Logf,
	}
}

// Pay implements replay.Gateway: wait for the payment created against taskID,
// then confirm it by POSTing a success webhook. amount/currency are read from
// the payment record so they match (the handler validates them).
func (g *mockGateway) Pay(ctx context.Context, taskID, method, status string, timeout time.Duration) error {
	cfg, ok := g.configs[method]
	if !ok {
		return fmt.Errorf("mock-gateway: no config for payment method %q", method)
	}

	tx, err := g.awaitReference(ctx, taskID, timeout)
	if err != nil {
		return err
	}

	g.logf("mock-gateway[%s]: confirming payment ref=%s amount=%s %s (task %s)", cfg.ID, tx.ReferenceNumber, tx.Amount.String(), tx.Currency, taskID)

	identityFields, err := resolveIdentityFields(cfg, tx)
	if err != nil {
		return err
	}

	// GovPay-shaped webhook envelope (mirrors integration/payment/govpay_test.go's
	// updateBody) — the only wire format this harness's mock speaks today.
	// cfg.IdentityFields overlays whatever extra fields this gateway's webhook
	// needs to prove which of its own services the payment belongs to.
	fields := map[string]any{
		"transactionID": "e2e-gw-tx",
		"serviceName":   "Application Fee",
		"data": []map[string]any{
			{"seq": "1", "paramName": "refNo", "value": tx.ReferenceNumber},
			{"seq": "2", "paramName": "status", "value": status},
			{"seq": "3", "paramName": "amount", "value": tx.Amount.String()},
			{"seq": "4", "paramName": "currency", "value": tx.Currency},
		},
	}
	for wireField, value := range identityFields {
		fields[wireField] = value
	}

	body, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("mock-gateway: marshal webhook: %w", err)
	}

	url := g.base + cfg.WebhookPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer := g.bearers[cfg.ID]; bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
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

// awaitReference polls the payment store until taskID's transaction has been
// assigned a gateway reference number (set when the checkout session is
// created), or timeout elapses.
func (g *mockGateway) awaitReference(ctx context.Context, taskID string, timeout time.Duration) (*payment.PaymentTransaction, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(gatewayPollInterval)
	defer ticker.Stop()

	for {
		tx, err := g.repo.GetByTaskID(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("mock-gateway: lookup payment for task %s: %w", taskID, err)
		}
		if tx != nil && tx.ReferenceNumber != "" {
			return tx, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("mock-gateway: no payment with a reference for task %s within %s", taskID, timeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// resolveIdentityFields looks up, for each wire field cfg.IdentityFields
// declares, the expected value from the transaction's own gateway metadata —
// the same metadata the fee's plugin_properties.gateway_metadata declared when
// the checkout session was created (see internal/tasks/plugins/payment.go).
// Echoing these back proves the mock is confirming the payment as the service
// it actually belongs to, whatever identity scheme this gateway uses. A
// gateway with no such scheme (cfg.IdentityFields empty) needs none of this.
func resolveIdentityFields(cfg PaymentConfig, tx *payment.PaymentTransaction) (map[string]string, error) {
	if len(cfg.IdentityFields) == 0 {
		return nil, nil
	}
	fields := make(map[string]string, len(cfg.IdentityFields))
	for wireField, metadataKey := range cfg.IdentityFields {
		value := tx.GatewayMetadata[metadataKey]
		if value == "" {
			return nil, fmt.Errorf("mock-gateway[%s]: payment %s has no %q in gateway metadata (required by configs/payments/%s.json's identityFields)",
				cfg.ID, tx.ReferenceNumber, metadataKey, cfg.ID)
		}
		fields[wireField] = value
	}
	return fields, nil
}
