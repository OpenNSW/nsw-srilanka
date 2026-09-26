package govpay

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// -----------------------------------------------------------------------------
// GovPay+ data encryption (spec §3, "Security Standards for Data Encryption")
//
// GovPay+ generates a 32-character AES-256 transaction key per call, RSA-OAEP
// encrypts it to this GO's public key in the "TransactionKey" header, and
// AES-CBC encrypts every field of each request data[] item under it. This GO
// decrypts the key, decrypts the request, and encrypts every field of the
// response with the same key so GovPay+ can read it back.
//
// Algorithms (spec §3.2): RSA/OAEP(SHA-256)/MGF1(SHA-256)/2048 for the
// transaction key; AES-256-CBC with PKCS7 padding for the payload.
// -----------------------------------------------------------------------------

// aesKeyLen is the length, in bytes, of the plaintext AES-256 transaction key
// (a "32-character" key per spec §3.1).
const aesKeyLen = 32

// ivLen is the AES-CBC IV length. The IV is the first 16 bytes of the derived
// AES key — see newCBC.
const ivLen = 16

// transactionKeyHeader is the HTTP header GovPay+ carries the RSA-encrypted
// transaction key in.
const transactionKeyHeader = "TransactionKey"

// ErrEncryptionNotConfigured reports a deployment that has not been given the
// GO's RSA private key. Like ErrWebhookClientNotConfigured it is an operational
// fault rather than evidence about the caller, so it must never be reported as
// a verification failure.
var ErrEncryptionNotConfigured = errors.New("govpay encryption not configured")

// ErrTransactionKeyMissing reports a GovPay+ call that carried no
// TransactionKey header. Every call is encrypted, so a call without one cannot
// be read at all.
var ErrTransactionKeyMissing = errors.New("govpay transaction key missing")

// ErrTransactionKeyInvalid reports a TransactionKey header this GO could not
// decrypt with its private key — the wrong key pair, a corrupted header, or a
// caller that is not GovPay+.
var ErrTransactionKeyInvalid = errors.New("govpay transaction key invalid")

// Decryptor holds the GO's RSA private key and performs the GovPay+ side of the
// encryption scheme: decrypt the transaction key, decrypt request fields,
// encrypt response fields.
type Decryptor struct {
	priv *rsa.PrivateKey
}

// newDecryptor builds a Decryptor from a PEM-encoded RSA private key
// (PKCS#8 or PKCS#1).
func newDecryptor(pemData []byte) (*Decryptor, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("govpay private key: no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return &Decryptor{priv: key}, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse govpay private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("govpay private key is not RSA")
	}
	return &Decryptor{priv: key}, nil
}

// loadDecryptor resolves the private key from an inline PEM first, then from a
// file path. It returns nil without error when neither is configured, so a
// deployment that has not enabled encryption still builds — the missing key
// surfaces as ErrEncryptionNotConfigured on the first call, naming the cause,
// rather than as a nil dereference.
func loadDecryptor(inlinePEM, path string) (*Decryptor, error) {
	if pemData := strings.TrimSpace(inlinePEM); pemData != "" {
		return newDecryptor([]byte(pemData))
	}
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read govpay private key (%s): %w", path, err)
	}
	return newDecryptor(data)
}

// decryptTransactionKey decrypts the base64 RSA-OAEP "TransactionKey" header
// into the 32-byte AES key.
//
// A header that does not decrypt is reported as ErrTransactionKeyInvalid: it is
// positive evidence the caller does not hold the matching public key, which the
// callback paths translate into a rejection rather than a transient failure.
func (d *Decryptor) decryptTransactionKey(header string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header))
	if err != nil {
		return nil, fmt.Errorf("%w: not base64: %v", ErrTransactionKeyInvalid, err)
	}
	key, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, d.priv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: rsa decrypt failed: %v", ErrTransactionKeyInvalid, err)
	}
	if len(key) != aesKeyLen {
		return nil, fmt.Errorf("%w: key must be %d bytes, got %d", ErrTransactionKeyInvalid, aesKeyLen, len(key))
	}
	return key, nil
}

// decryptParams decrypts every field (seq, paramName, value) of each request
// data[] item in place. Each arrives as a base64 AES-CBC ciphertext string and
// is replaced by its plaintext (spec §3.1: GovPay+ encrypts all three).
//
// Values are left as strings rather than re-parsed into numbers: ciphertext is
// always a string on the wire, so the original JSON type is not recoverable.
// paramDecimal already accepts a string, so amounts still convert exactly.
func decryptParams(params []govPayParam, key []byte) error {
	for i := range params {
		p := &params[i]

		var err error
		if p.Seq, err = aesCBCDecrypt(key, p.Seq); err != nil {
			return fmt.Errorf("decrypt seq: %w", err)
		}
		if p.ParamName, err = aesCBCDecrypt(key, p.ParamName); err != nil {
			return fmt.Errorf("decrypt paramName: %w", err)
		}

		ciphertext, ok := p.Value.(string)
		if !ok {
			return fmt.Errorf("encrypted value for %q must be a string, got %T", p.ParamName, p.Value)
		}
		if p.Value, err = aesCBCDecrypt(key, ciphertext); err != nil {
			return fmt.Errorf("decrypt value for %q: %w", p.ParamName, err)
		}
	}
	return nil
}

// encryptResponseObjects AES-CBC encrypts every string reachable from objs, in
// place, so GovPay+ can decrypt the response (spec §3.1.7).
//
// The response objects are string-typed throughout — every field is encrypted,
// and AES output is base64 text — so the walk takes whole objects rather than a
// hand-listed set of fields. A field added to PresentmentObject or PaymentItem
// is then encrypted by construction instead of being silently sent in clear.
// Empty fields encrypt to the ciphertext of "", keeping the wire shape uniform.
func encryptResponseObjects[T any](objs []T, key []byte) error {
	return encryptStrings(reflect.ValueOf(objs), key)
}

func encryptStrings(v reflect.Value, key []byte) error {
	switch v.Kind() {
	case reflect.String:
		enc, err := aesCBCEncrypt(key, v.String())
		if err != nil {
			return err
		}
		v.SetString(enc)
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return encryptStrings(v.Elem(), key)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := encryptStrings(v.Index(i), key); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := range v.NumField() {
			// An unexported field cannot be set, and none of the response
			// types have one; skipping keeps the walk from panicking if that
			// ever changes.
			if v.Type().Field(i).IsExported() {
				if err := encryptStrings(v.Field(i), key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// aesCBCEncrypt encrypts plaintext with AES-256-CBC and PKCS7 padding, using
// IV = derivedKey[:16] (spec §3.2.2), and returns the base64 ciphertext.
func aesCBCEncrypt(key []byte, plaintext string) (string, error) {
	block, iv, err := newCBC(key)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plaintext), block.BlockSize())
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// aesCBCDecrypt reverses aesCBCEncrypt.
func aesCBCDecrypt(key []byte, b64 string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("value not base64: %w", err)
	}
	block, iv, err := newCBC(key)
	if err != nil {
		return "", err
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return "", fmt.Errorf("ciphertext is not a whole number of blocks")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext, block.BlockSize())
}

// newCBC returns an AES-256 cipher block keyed with SHA-256(txnKey), plus the
// IV (the first 16 bytes of that derived key).
//
// Deriving the key by hashing is what GovPay+ does in practice (verified
// against a live GovPay+ request), even though the spec reads as if the
// transaction key were used directly.
func newCBC(txnKey []byte) (cipher.Block, []byte, error) {
	if len(txnKey) != aesKeyLen {
		return nil, nil, fmt.Errorf("aes key must be %d bytes, got %d", aesKeyLen, len(txnKey))
	}
	key := sha256.Sum256(txnKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, nil, err
	}
	return block, key[:ivLen], nil
}

// pkcs7Pad appends PKCS7 padding so the data is a whole number of blocks.
func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

// pkcs7Unpad removes and validates PKCS7 padding.
func pkcs7Unpad(data []byte, blockSize int) (string, error) {
	n := len(data)
	if n == 0 || n%blockSize != 0 {
		return "", fmt.Errorf("invalid padded length")
	}
	pad := int(data[n-1])
	if pad == 0 || pad > blockSize {
		return "", fmt.Errorf("invalid padding")
	}
	for _, b := range data[n-pad:] {
		if int(b) != pad {
			return "", fmt.Errorf("invalid padding")
		}
	}
	return string(data[:n-pad]), nil
}
