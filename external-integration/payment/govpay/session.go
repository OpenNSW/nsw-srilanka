package govpay

import (
	"context"
	"fmt"

	corepayment "github.com/OpenNSW/core/payment"
)

// transactionKey recovers the per-call AES key for the request currently being
// served: it reads the RSA-encrypted TransactionKey header off the inbound
// request and decrypts it with this GO's private key.
//
// The header is not among the parameters core/payment passes to
// ExtractReferenceNumber or HandleValidateReference, and VerifyWebhook cannot
// hand anything forward (it returns only an error, and the context it is given
// is not propagated). RequestFromContext is the mechanism core/payment provides
// for exactly this: it attaches the inbound *http.Request to the context on
// both callback paths, before any gateway method runs.
//
// The cost is one RSA decrypt per entry point rather than one per request.
// That is a sub-millisecond operation on a 2048-bit key, and it keeps each
// entry point independently correct instead of relying on ordering between
// interface methods.
func (g *GovPayGateway) transactionKey(ctx context.Context) ([]byte, error) {
	if g.decryptor == nil {
		return nil, fmt.Errorf("govpay: cannot read encrypted call: %w", ErrEncryptionNotConfigured)
	}

	r := corepayment.RequestFromContext(ctx)
	if r == nil {
		// PaymentService was invoked directly, bypassing HTTPHandler. Real
		// traffic always carries a request; this is a wiring fault, not a
		// statement about the caller.
		return nil, fmt.Errorf("govpay: no inbound request on context; cannot read %s header", transactionKeyHeader)
	}

	header := r.Header.Get(transactionKeyHeader)
	if header == "" {
		return nil, fmt.Errorf("govpay: %w", ErrTransactionKeyMissing)
	}
	return g.decryptor.decryptTransactionKey(header)
}

// decryptRequest parses a GovPay+ call and decrypts its data[] items, returning
// the plaintext request together with the AES key the response must be
// encrypted with.
func (g *GovPayGateway) decryptRequest(ctx context.Context, raw []byte) (govPayRequest, []byte, error) {
	req, err := parseGovPayRequest(raw)
	if err != nil {
		return govPayRequest{}, nil, err
	}

	key, err := g.transactionKey(ctx)
	if err != nil {
		return govPayRequest{}, nil, err
	}
	if err := decryptParams(req.Data, key); err != nil {
		return govPayRequest{}, nil, err
	}
	return req, key, nil
}
