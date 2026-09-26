package govpay

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// govPayPlusVectors are ciphertexts produced by the GovPay+ encryptor itself
// (external-integrations-sandbox/govpay/server/crypto.go), using the 32-character
// transaction key below. They pin this GO's AES layer to the counterpart
// implementation: key = SHA-256(transaction key), IV = the first 16 bytes of
// that derived key, AES-256-CBC with PKCS7 padding, base64-encoded.
//
// Self-consistent round-trip tests cannot catch a shared misreading of the
// spec — if both sides derived the key the same wrong way, they would still
// agree with themselves. These vectors are the check that actually binds the
// two implementations together, and they will fail loudly if either side's
// derivation drifts.
//
// No key material is secret here: the transaction key is a fixed test literal,
// and RSA is deliberately not covered (an RSA vector would require committing
// a private key, and OAEP is randomised so its ciphertext cannot be pinned).
var govPayPlusVectors = map[string]string{
	"1":            "g7vauhm9iSHq1beXqv9neQ==",
	"refNo":        "uHW5CQOGOx8Xe/QzXyvITg==",
	"TNSWINTEROP1": "7yoD6eZfqMHPPSkV2DdI+w==",
	"1500.00":      "CArW58ZxYNiRwx7SqhI9MQ==",
	"":             "a3HE2swh2ikintPoe3hKQw==",
	"SUCCESS":      "dRSM+ZIQ/wU6gDKzP7ChxQ==",
}

// govPayPlusVectorKey is the transaction key those vectors were produced with.
var govPayPlusVectorKey = []byte(strings.Repeat("A", aesKeyLen))

func TestDecryptsGovPayPlusCiphertext(t *testing.T) {
	for plain, cipherText := range govPayPlusVectors {
		got, err := aesCBCDecrypt(govPayPlusVectorKey, cipherText)
		require.NoErrorf(t, err, "decrypting vector for %q", plain)
		assert.Equalf(t, plain, got, "vector for %q", plain)
	}
}

// The scheme is deterministic (the IV is derived, not random), so this GO must
// reproduce GovPay+'s ciphertext byte for byte — which is what lets GovPay+
// decrypt the responses this GO encrypts.
func TestProducesGovPayPlusCiphertext(t *testing.T) {
	for plain, want := range govPayPlusVectors {
		got, err := aesCBCEncrypt(govPayPlusVectorKey, plain)
		require.NoErrorf(t, err, "encrypting %q", plain)
		assert.Equalf(t, want, got, "ciphertext for %q", plain)
	}
}
