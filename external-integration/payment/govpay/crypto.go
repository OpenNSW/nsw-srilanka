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
	"strings"
)

// -----------------------------------------------------------------------------
// GovPay+ data encryption (spec §3, "Security Standards for Data Encryption")
//
// GovPay+ encrypts every presentment/update call it makes to this GO:
//
//   - It generates a 32-character AES-256 transaction key per call, RSA-OAEP
//     encrypts it with this GO's public key, and sends it base64-encoded in the
//     "TransactionKey" HTTP header.
//   - Every field of each request data[] item (seq, paramName, value) is
//     AES-256-CBC encrypted with that transaction key, base64-encoded.
//
// This GO decrypts the transaction key with its RSA private key, decrypts each
// request field, and encrypts every field of the response with the same AES key
// so GovPay+ can read it back.
//
// Algorithm standards (spec §3.2):
//   - TransactionKey: RSA / OAEP (SHA-256) / MGF1(SHA-256) / 2048-bit.
//   - Payload:        AES / CBC / 256-bit, PKCS7 padding, with
//     AES key = SHA-256(transaction key) and IV = the first 16 bytes of that
//     derived key.
// -----------------------------------------------------------------------------

// aesKeyLen is the length, in bytes, of the plaintext AES-256 transaction key
// (a "32-character" key per spec §3.1).
const aesKeyLen = 32

// ivLen is the AES-CBC IV length. The IV is the first 16 bytes of the derived
// AES key, SHA-256(transaction key) — see deriveAESKey.
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
	priv, err := parseRSAPrivateKey(pemData)
	if err != nil {
		return nil, err
	}
	return &Decryptor{priv: priv}, nil
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

func parseRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("govpay private key: no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse govpay private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("govpay private key is not RSA")
	}
	return key, nil
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
		seq, err := aesCBCDecrypt(key, params[i].Seq)
		if err != nil {
			return fmt.Errorf("decrypt seq: %w", err)
		}
		params[i].Seq = seq

		name, err := aesCBCDecrypt(key, params[i].ParamName)
		if err != nil {
			return fmt.Errorf("decrypt paramName: %w", err)
		}
		params[i].ParamName = name

		ciphertext, ok := params[i].Value.(string)
		if !ok {
			return fmt.Errorf("encrypted value for %q must be a string, got %T",
				params[i].ParamName, params[i].Value)
		}
		plain, err := aesCBCDecrypt(key, ciphertext)
		if err != nil {
			return fmt.Errorf("decrypt value for %q: %w", params[i].ParamName, err)
		}
		params[i].Value = plain
	}
	return nil
}

// encryptPresentmentObjects encrypts every field of every presentment object so
// GovPay+ can decrypt them (spec §3.1.7). Empty fields are encrypted as empty
// strings, keeping the wire shape uniform.
func encryptPresentmentObjects(objs []PresentmentObject, key []byte) error {
	for i := range objs {
		o := &objs[i]
		if err := encryptFields(key,
			&o.ObjType, &o.Seq, &o.ID, &o.Placeholder, &o.InitialValue,
			&o.DataType, &o.MaxLength, &o.SelectionType, &o.Mask, &o.NotNull,
			&o.Enabled, &o.Returned, &o.Rows, &o.Cols, &o.ReturnParam,
			&o.IsPaymentReference, &o.IsPaymentAmount, &o.ReturnValue,
		); err != nil {
			return err
		}
		if err := encryptObjExtras(key, o.ObjData, o.TableData); err != nil {
			return err
		}
	}
	return nil
}

// encryptPaymentItems is encryptPresentmentObjects for the update receipt.
func encryptPaymentItems(items []PaymentItem, key []byte) error {
	for i := range items {
		it := &items[i]
		if err := encryptFields(key,
			&it.ObjType, &it.Seq, &it.ID, &it.Placeholder, &it.InitialValue,
			&it.DataType, &it.MaxLength, &it.SelectionType, &it.Mask,
			&it.NotNull, &it.Enabled, &it.Returned, &it.Rows, &it.Cols,
			&it.ReturnParam, &it.ReturnValue,
		); err != nil {
			return err
		}
		if err := encryptObjExtras(key, nil, it.TableData); err != nil {
			return err
		}
	}
	return nil
}

// encryptObjExtras encrypts the nested string fields of any combo items and
// table data attached to a response object. These are empty in the current fee
// flows, but are handled so that "every field is encrypted" holds for combo and
// table objects too.
func encryptObjExtras(key []byte, objData []ComboItem, table *TableDataObject) error {
	for j := range objData {
		if err := encryptFields(key, &objData[j].ID, &objData[j].Data); err != nil {
			return err
		}
	}
	if table == nil {
		return nil
	}
	for j := range table.Header {
		if err := encryptFields(key, &table.Header[j].DataType, &table.Header[j].Value, &table.Header[j].Enabled); err != nil {
			return err
		}
	}
	for j := range table.RowData {
		if err := encryptFields(key, &table.RowData[j].DataType, &table.RowData[j].Value, &table.RowData[j].Enabled); err != nil {
			return err
		}
	}
	return nil
}

// encryptFields AES-CBC encrypts each referenced string in place.
func encryptFields(key []byte, fields ...*string) error {
	for _, f := range fields {
		enc, err := aesCBCEncrypt(key, *f)
		if err != nil {
			return err
		}
		*f = enc
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
	unpadded, err := pkcs7Unpad(plaintext, block.BlockSize())
	if err != nil {
		return "", err
	}
	return string(unpadded), nil
}

// deriveAESKey turns the 32-character transaction key into the actual AES-256
// key: SHA-256 of the transaction key bytes. This is what GovPay+ does in
// practice (verified against a live GovPay+ request), even though the spec
// reads as if the transaction key were used directly.
func deriveAESKey(txnKey []byte) []byte {
	sum := sha256.Sum256(txnKey)
	return sum[:]
}

// newCBC returns an AES-256 cipher block keyed with SHA-256(txnKey), plus the
// IV (the first 16 bytes of that derived key).
func newCBC(txnKey []byte) (cipher.Block, []byte, error) {
	if len(txnKey) != aesKeyLen {
		return nil, nil, fmt.Errorf("aes key must be %d bytes, got %d", aesKeyLen, len(txnKey))
	}
	key := deriveAESKey(txnKey)
	block, err := aes.NewCipher(key)
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
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	n := len(data)
	if n == 0 || n%blockSize != 0 {
		return nil, fmt.Errorf("invalid padded length")
	}
	pad := int(data[n-1])
	if pad == 0 || pad > blockSize {
		return nil, fmt.Errorf("invalid padding")
	}
	for _, b := range data[n-pad:] {
		if int(b) != pad {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:n-pad], nil
}
