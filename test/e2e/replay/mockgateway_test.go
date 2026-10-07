package replay_e2e

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
// knowledge of any specific gateway. A gateway that declares privateKeyField
// has its webhook encrypted to the key pair the harness minted for it. It
// implements replay.PaymentGateway.
//
// The reference is only rendered into the task's markdown view, so the mock
// reads it from the payment table rather than over HTTP. The payment service
// does not record which task a payment is for; the payment plugin puts it in
// the checkout metadata (gateway_metadata.task_id), which is what the mock
// matches on.
type mockGateway struct {
	db      *gorm.DB
	client  *http.Client
	base    string // the in-process NSW app base URL; set by the harness after start
	configs map[string]PaymentConfig
	bearers map[string]string          // paymentID -> SERVICE bearer token (empty = unauthenticated)
	keys    map[string]*rsa.PrivateKey // paymentID -> key pair the webhook is encrypted to (absent = plaintext)
	logf    func(string, ...any)
}

func newMockGateway(t *testing.T, db *gorm.DB, configs []PaymentConfig, keys map[string]*rsa.PrivateKey) *mockGateway {
	t.Helper()
	cfgMap := make(map[string]PaymentConfig, len(configs))
	for _, c := range configs {
		cfgMap[c.ID] = c
	}
	return &mockGateway{
		db:      db,
		client:  &http.Client{Timeout: 10 * time.Second},
		configs: cfgMap,
		bearers: make(map[string]string),
		keys:    keys,
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
	data := []map[string]string{
		{"seq": "1", "paramName": "refNo", "value": tx.ReferenceNumber},
		{"seq": "2", "paramName": "status", "value": status},
		{"seq": "3", "paramName": "amount", "value": tx.Amount.String()},
		{"seq": "4", "paramName": "currency", "value": tx.Currency},
	}
	var transactionKeyHeader string
	if key, ok := g.keys[cfg.ID]; ok {
		if transactionKeyHeader, err = encryptAsGovPay(&key.PublicKey, data); err != nil {
			return fmt.Errorf("mock-gateway[%s]: encrypt webhook: %w", cfg.ID, err)
		}
	}
	fields := map[string]any{
		"transactionID": "e2e-gw-tx",
		"serviceName":   "Application Fee",
		"data":          data,
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
	if transactionKeyHeader != "" {
		req.Header.Set("TransactionKey", transactionKeyHeader)
	}
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
		tx, err := g.latestPaymentFor(ctx, taskID)
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

// latestPaymentFor returns taskID's newest payment transaction, or nil if there
// is none yet. A looped payment step can leave more than one, and the newest is
// the one the trader is paying now.
func (g *mockGateway) latestPaymentFor(ctx context.Context, taskID string) (*payment.PaymentTransaction, error) {
	var tx payment.PaymentTransaction
	err := g.db.WithContext(ctx).
		Where("gateway_metadata->>'task_id' = ?", taskID).
		Order("created_at DESC").
		Limit(1).
		Find(&tx).Error
	if err != nil {
		return nil, err
	}
	if tx.ID == "" {
		return nil, nil
	}
	return &tx, nil
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

// encryptAsGovPay encrypts every field of each data[] item in place the way
// GovPay+ does (spec §3): a fresh 32-character transaction key, AES-256-CBC
// with PKCS7 padding under key SHA-256(transaction key) and IV its first 16
// bytes. It returns the transaction key RSA-OAEP(SHA-256) encrypted to pub and
// base64-encoded, the value of the TransactionKey header.
func encryptAsGovPay(pub *rsa.PublicKey, data []map[string]string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	transactionKey := []byte(hex.EncodeToString(raw))

	derived := sha256.Sum256(transactionKey)
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return "", err
	}
	for _, item := range data {
		for field, plaintext := range item {
			pad := block.BlockSize() - len(plaintext)%block.BlockSize()
			padded := append([]byte(plaintext), bytes.Repeat([]byte{byte(pad)}, pad)...)
			ciphertext := make([]byte, len(padded))
			cipher.NewCBCEncrypter(block, derived[:block.BlockSize()]).CryptBlocks(ciphertext, padded)
			item[field] = base64.StdEncoding.EncodeToString(ciphertext)
		}
	}

	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, transactionKey, nil)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encryptedKey), nil
}
