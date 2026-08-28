// Package webhook receives what SLPA's Cargo Management System sends back: the
// calls it makes as a service order moves through its approvals, and as the
// invoice raised against it is issued and paid. Outbound calls to the CMS live in
// the ecdn and serviceorder packages.
//
// Nothing here writes to a task record directly. An event resumes the step
// waiting on it through the task manager, and the task workflow decides whether
// that step opens again — which is how an event that does not end a lifecycle
// still reaches the trader.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// SignatureHeader carries the CMS's HMAC over the request body.
const SignatureHeader = "X-Signature"

// signaturePrefix names the digest the header carries, as GitHub's convention
// does, so a change of algorithm is visible in the header rather than silent.
const signaturePrefix = "sha256="

// ErrUnsigned is returned when a request carries no signature at all.
var ErrUnsigned = errors.New("slpa webhook: request is not signed")

// ErrBadSignature is returned when the signature does not match the body.
var ErrBadSignature = errors.New("slpa webhook: signature does not match the body")

// VerifySignature reports whether header authenticates body under secret.
//
// The digest is taken over the exact bytes received — not over a re-encoding of
// the decoded JSON, which would differ from what the CMS signed in key order,
// whitespace and number formatting alone. That is why the handler reads the body
// before it decodes anything.
//
// The comparison is constant-time: a byte-by-byte one leaks, through timing,
// which prefix of a forged signature was right, and a signature is guessable one
// byte at a time if it does.
func VerifySignature(header string, body []byte, secret string) error {
	header = strings.TrimSpace(header)
	if header == "" {
		return ErrUnsigned
	}

	digest, ok := strings.CutPrefix(header, signaturePrefix)
	if !ok {
		return fmt.Errorf("%w: expected the %s prefix", ErrBadSignature, strings.TrimSuffix(signaturePrefix, "="))
	}

	sent, err := hex.DecodeString(digest)
	if err != nil {
		return fmt.Errorf("%w: not hex", ErrBadSignature)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(sent, mac.Sum(nil)) {
		return ErrBadSignature
	}
	return nil
}

// Sign returns the header value for body under secret. The webhook's sender is
// SLPA, so this exists for tests and for anyone reproducing a call by hand.
func Sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}
