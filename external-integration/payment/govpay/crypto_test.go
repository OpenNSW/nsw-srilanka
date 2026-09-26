package govpay

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKeyPair is an ephemeral RSA key generated once per test run. Real key
// material is environment-specific and never committed, so the tests mint their
// own pair rather than depending on files that are absent in a fresh clone.
var (
	testKeyOnce sync.Once
	testKey     *rsa.PrivateKey
)

func testKeyPair(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	return testRSAKey()
}

// testRSAKey is testKeyPair without a *testing.T, for the zero-argument
// gateway constructors the existing tests use.
func testRSAKey() *rsa.PrivateKey {
	testKeyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testKey = k
	})
	return testKey
}

// testDecryptor builds a Decryptor over the ephemeral test key.
func testDecryptor() *Decryptor {
	return &Decryptor{priv: testRSAKey()}
}

// newTestGateway is a gateway that can read encrypted calls but enforces no
// identity — the shape most parsing/validation tests want.
func newTestGateway() *GovPayGateway {
	return &GovPayGateway{decryptor: testDecryptor()}
}

// -----------------------------------------------------------------------------
// Encrypted-call wrappers
//
// Every GovPay+ call is encrypted, so tests drive the real entry points through
// these rather than passing plaintext. Each encrypts the body, attaches the
// TransactionKey header to the context the way HTTPHandler does, and calls the
// method under test.
// -----------------------------------------------------------------------------

func (g *GovPayGateway) parseWebhookEnc(t *testing.T, body []byte, headers map[string][]string) (*corepayment.WebhookPayload, *corepayment.WebhookResponse, error) {
	t.Helper()
	ctx, enc := encryptAsGovPay(t, body)
	return g.ParseWebhook(ctx, enc, headers)
}

func (g *GovPayGateway) validateEnc(t *testing.T, tx *corepayment.ValidationTransaction, isPayable bool, body []byte) (*corepayment.ValidationResponse, error) {
	t.Helper()
	ctx, enc := encryptAsGovPay(t, body)
	return g.HandleValidateReference(ctx, tx, isPayable, enc)
}

func (g *GovPayGateway) extractRefEnc(t *testing.T, body []byte) (string, error) {
	t.Helper()
	ctx, enc := encryptAsGovPay(t, body)
	return g.ExtractReferenceNumber(ctx, enc)
}

// testPrivateKeyPEM renders the ephemeral key as a PKCS#8 PEM, so tests drive
// the same parsing path production config does.
func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(testKeyPair(t))
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// encryptAsGovPay plays the GovPay+ side: it takes a plaintext request body,
// encrypts every data[] field with a fresh transaction key, RSA-encrypts that
// key to this GO's public key, and returns the encrypted body plus a context
// carrying the TransactionKey header the way HTTPHandler would.
func encryptAsGovPay(t *testing.T, plaintextBody []byte) (context.Context, []byte) {
	t.Helper()

	aesKey := []byte(strings.Repeat("K", aesKeyLen))

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(plaintextBody, &payload); err != nil {
		// Malformed input is passed through untouched so the parser still
		// sees exactly what the test wrote.
		return ctxWithTransactionKey(t, aesKey), plaintextBody
	}

	if rawData, ok := payload["data"]; ok {
		var params []govPayParam
		require.NoError(t, json.Unmarshal(rawData, &params))
		for i := range params {
			seq, err := aesCBCEncrypt(aesKey, params[i].Seq)
			require.NoError(t, err)
			params[i].Seq = seq

			name, err := aesCBCEncrypt(aesKey, params[i].ParamName)
			require.NoError(t, err)
			params[i].ParamName = name

			val, err := aesCBCEncrypt(aesKey, paramValueString(params[i].Value))
			require.NoError(t, err)
			params[i].Value = val
		}
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		payload["data"] = encoded
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return ctxWithTransactionKey(t, aesKey), body
}

// ctxWithTransactionKey RSA-encrypts aesKey to the test public key and returns
// a context carrying it in the TransactionKey header, as HTTPHandler would.
func ctxWithTransactionKey(t *testing.T, aesKey []byte) context.Context {
	t.Helper()
	header, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &testRSAKey().PublicKey, aesKey, nil)
	require.NoError(t, err)

	r := httptest.NewRequest("POST", "/api/v1/payments/govpay/validate", nil)
	r.Header.Set(transactionKeyHeader, base64.StdEncoding.EncodeToString(header))
	return corepayment.ContextWithRequest(context.Background(), r)
}

// decryptAsGovPay reverses the field encryption on a response, so assertions can
// be written against plaintext.
func decryptAsGovPay(t *testing.T, ciphertext string) string {
	t.Helper()
	plain, err := aesCBCDecrypt([]byte(strings.Repeat("K", aesKeyLen)), ciphertext)
	require.NoError(t, err)
	return plain
}

func TestAESCBCRoundTrip(t *testing.T) {
	key := []byte(strings.Repeat("K", aesKeyLen))
	for _, plain := range []string{"", "a", "refNo", "TNSW1", strings.Repeat("x", 16), strings.Repeat("y", 31), "1500.00"} {
		enc, err := aesCBCEncrypt(key, plain)
		require.NoError(t, err)
		got, err := aesCBCDecrypt(key, enc)
		require.NoError(t, err)
		assert.Equal(t, plain, got)
	}
}

func TestAESCBCDerivesKeyAndIV(t *testing.T) {
	// The AES key is SHA-256(transaction key) and the IV its first 16 bytes.
	// Pinning this guards the one detail that must match GovPay+ exactly.
	key := []byte(strings.Repeat("K", aesKeyLen))
	block, iv, err := newCBC(key)
	require.NoError(t, err)
	sum := sha256.Sum256(key)
	assert.Equal(t, 16, block.BlockSize())
	assert.Equal(t, sum[:ivLen], iv)
}

func TestAESCBCRejectsWrongKeyLength(t *testing.T) {
	_, err := aesCBCEncrypt([]byte("short"), "x")
	require.Error(t, err)
}

func TestDecryptTransactionKeyRoundTrip(t *testing.T) {
	d, err := newDecryptor([]byte(testPrivateKeyPEM(t)))
	require.NoError(t, err)

	aesKey := []byte(strings.Repeat("Z", aesKeyLen))
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &testKeyPair(t).PublicKey, aesKey, nil)
	require.NoError(t, err)

	got, err := d.decryptTransactionKey(base64.StdEncoding.EncodeToString(ciphertext))
	require.NoError(t, err)
	assert.Equal(t, aesKey, got)
}

// A plaintext key is exactly what this GO used to receive before the scheme
// existed, so rejecting it is the behaviour that proves the upgrade took.
func TestDecryptTransactionKeyRejectsPlaintext(t *testing.T) {
	d, err := newDecryptor([]byte(testPrivateKeyPEM(t)))
	require.NoError(t, err)

	_, err = d.decryptTransactionKey(strings.Repeat("K", aesKeyLen))
	require.ErrorIs(t, err, ErrTransactionKeyInvalid)
}

func TestDecryptTransactionKeyRejectsGarbage(t *testing.T) {
	d, err := newDecryptor([]byte(testPrivateKeyPEM(t)))
	require.NoError(t, err)

	_, err = d.decryptTransactionKey("!!!not base64!!!")
	require.ErrorIs(t, err, ErrTransactionKeyInvalid)
}

func TestLoadDecryptor(t *testing.T) {
	t.Run("inline pem", func(t *testing.T) {
		d, err := loadDecryptor(testPrivateKeyPEM(t), "")
		require.NoError(t, err)
		require.NotNil(t, d)
	})

	t.Run("neither configured yields nil, not an error", func(t *testing.T) {
		d, err := loadDecryptor("", "")
		require.NoError(t, err)
		assert.Nil(t, d)
	})

	t.Run("unreadable file is an error", func(t *testing.T) {
		_, err := loadDecryptor("", "/nonexistent/go_private.pem")
		require.Error(t, err)
	})

	t.Run("malformed pem is an error", func(t *testing.T) {
		_, err := loadDecryptor("not a pem", "")
		require.Error(t, err)
	})
}

// An unconfigured gateway must name the missing key rather than panic on a nil
// decryptor, and must not look like a caller problem.
func TestTransactionKeyWithoutConfiguredKey(t *testing.T) {
	g := &GovPayGateway{}
	_, err := g.transactionKey(context.Background())
	require.ErrorIs(t, err, ErrEncryptionNotConfigured)
}

func TestTransactionKeyMissingHeader(t *testing.T) {
	g := newTestGateway()
	r := httptest.NewRequest("POST", "/api/v1/payments/govpay/validate", nil)
	ctx := corepayment.ContextWithRequest(context.Background(), r)

	_, err := g.transactionKey(ctx)
	require.ErrorIs(t, err, ErrTransactionKeyMissing)
}

func TestDecryptParamsRejectsNonStringValue(t *testing.T) {
	params := []govPayParam{{Seq: "1", ParamName: "refNo", Value: 42}}
	err := decryptParams(params, []byte(strings.Repeat("K", aesKeyLen)))
	require.Error(t, err)
}
