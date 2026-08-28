package webhook

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "a-secret-shared-with-slpa"

var body = []byte(`{"event":"service_order.approved_by_accountant","slug":"8d326f3a-643a-4a1d-8072-87130288b032"}`)

func TestVerifySignature(t *testing.T) {
	require.NoError(t, VerifySignature(Sign(body, secret), body, secret))
}

// The digest is over the bytes as received. Re-encoding the decoded JSON would
// differ from what SLPA signed in key order and spacing alone, so a body that
// means the same thing but reads differently must not verify.
func TestVerifySignature_CoversTheExactBytes(t *testing.T) {
	signature := Sign(body, secret)

	reordered := []byte(`{"slug":"8d326f3a-643a-4a1d-8072-87130288b032","event":"service_order.approved_by_accountant"}`)
	assert.ErrorIs(t, VerifySignature(signature, reordered, secret), ErrBadSignature)

	respaced := []byte(strings.ReplaceAll(string(body), `":"`, `": "`))
	assert.ErrorIs(t, VerifySignature(signature, respaced, secret), ErrBadSignature)
}

func TestVerifySignature_Rejects(t *testing.T) {
	valid := Sign(body, secret)

	for name, tc := range map[string]struct {
		header string
		secret string
		want   error
	}{
		"no signature at all":     {"", secret, ErrUnsigned},
		"whitespace only":         {"   ", secret, ErrUnsigned},
		"another party's secret":  {Sign(body, "not-the-secret"), secret, ErrBadSignature},
		"a tampered digest":       {valid[:len(valid)-1] + "0", secret, ErrBadSignature},
		"no algorithm prefix":     {strings.TrimPrefix(valid, "sha256="), secret, ErrBadSignature},
		"a different algorithm":   {"sha1=" + strings.TrimPrefix(valid, "sha256="), secret, ErrBadSignature},
		"a digest that is no hex": {"sha256=not-hex-at-all", secret, ErrBadSignature},
		"a truncated digest":      {"sha256=9f86d081", secret, ErrBadSignature},
	} {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, VerifySignature(tc.header, body, tc.secret), tc.want)
		})
	}
}

// A body of no bytes is still signed: an empty POST that verifies is a decision
// the sender takes, not something to wave through.
func TestVerifySignature_EmptyBody(t *testing.T) {
	require.NoError(t, VerifySignature(Sign(nil, secret), nil, secret))
	assert.ErrorIs(t, VerifySignature(Sign(body, secret), nil, secret), ErrBadSignature)
}
