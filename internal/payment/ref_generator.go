// Package payment issues the references payers pay against, from the refid
// formats in config.yaml. RefGenerator implements core/payment's
// ReferenceGenerator: a checkout takes the fee's own format when its metadata
// names one (ReferenceMetadata), and the default format otherwise.
package payment

import (
	"context"
	"fmt"
	"strings"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/refid"
)

// The default payment reference format, for fees without a format of their
// own. config.yaml defines it in its refid section.
const (
	DefaultReferenceIssuer = "TNSW"
	DefaultReferenceIDType = "payment_ref"
)

// Checkout metadata keys that carry a fee's reference format: the format's
// issuer and id_type, and one key per param, named metadataKeyParamPrefix plus
// the param's name. They are stored with the transaction.
const (
	metadataKeyIssuer      = "reference_issuer"
	metadataKeyIDType      = "reference_id_type"
	metadataKeyParamPrefix = "reference_param."
)

// ReferenceMetadata returns the checkout metadata that names a fee's reference
// format and the values of its params.
func ReferenceMetadata(issuer, idType string, params map[string]string) map[string]string {
	metadata := make(map[string]string, len(params)+2)
	metadata[metadataKeyIssuer] = issuer
	metadata[metadataKeyIDType] = idType
	for name, value := range params {
		metadata[metadataKeyParamPrefix+name] = value
	}
	return metadata
}

// IsReferenceMetadataKey reports whether key is one ReferenceMetadata writes.
func IsReferenceMetadataKey(key string) bool {
	return key == metadataKeyIssuer || key == metadataKeyIDType ||
		strings.HasPrefix(key, metadataKeyParamPrefix)
}

// RefGenerator issues payment references from the refid registry: in the format
// a checkout's metadata names (ReferenceMetadata), and in the default format
// (DefaultReferenceIssuer, DefaultReferenceIDType) for a checkout that names
// none.
type RefGenerator struct {
	refIDs refid.Registry
}

// NewRefGenerator builds the generator. refIDs must be non-nil.
func NewRefGenerator(refIDs refid.Registry) *RefGenerator {
	if refIDs == nil {
		panic("refIDs is nil")
	}
	return &RefGenerator{refIDs: refIDs}
}

// GenerateReference implements corepayment.ReferenceGenerator.
func (g *RefGenerator) GenerateReference(ctx context.Context, req corepayment.CreateCheckoutRequest) (string, error) {
	issuer, idType := req.Metadata[metadataKeyIssuer], req.Metadata[metadataKeyIDType]
	switch {
	case issuer == "" && idType == "":
		issuer, idType = DefaultReferenceIssuer, DefaultReferenceIDType
	case issuer == "" || idType == "":
		return "", fmt.Errorf("payment reference: the checkout names %s=%q and %s=%q; a format needs both",
			metadataKeyIssuer, issuer, metadataKeyIDType, idType)
	}
	params := make(map[string]string)
	for key, value := range req.Metadata {
		if name, ok := strings.CutPrefix(key, metadataKeyParamPrefix); ok {
			params[name] = value
		}
	}
	return g.refIDs.Generate(ctx, issuer, idType, params)
}
