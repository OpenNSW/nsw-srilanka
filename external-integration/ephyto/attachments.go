package ephyto

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto/spscert"
)

// Files reads an uploaded document out of this deployment's storage. It is the
// same contract the CusDec submission uses for its attachments, so one storage
// service satisfies both.
type Files interface {
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
}

// Attachment limits, taken from the ePhyto Guidelines (Mapping ISPM 12 to the
// ePhyto standard, v2.12, 14 May 2025), which the Hub validates on receipt:
//
//   - ram:AttachmentBinaryObject — "Format: JPG, GIF, PNG and PDF. MaxSize: 3MB"
//   - the certificate as a whole — "The max size of the certificate must not
//     exceed 104 MB"
//
// They are enforced here rather than left to the Hub because a refusal there
// arrives as a validation fault the trader never sees, while the same limit
// checked before the call names the file that is too large.
//
// The certificate ceiling is measured against the base64 text, not the file:
// encoding inflates a document by about a third and it is the encoded blob
// that sits in the XML the Hub sizes.
const (
	maxAttachmentBytes  = 3 * 1024 * 1024
	maxCertificateBytes = 104 * 1024 * 1024
)

// attachmentReadTimeout bounds reading the documents out of storage.
//
// The SOAP interpreter contract carries no context (BuildEnvelope takes only
// the operation and the inputs), so there is no caller deadline to inherit and
// this is the deadline instead. It is a deliberate stand-in: the proper fix is
// a context on that contract, at which point this constant goes away.
const attachmentReadTimeout = 30 * time.Second

// resolveAttachments replaces each attachment's storage key with the file's
// content, base64-encoded, and fills in the filename and MIME type the Hub
// expects alongside it.
//
// An attachment that already carries content is left alone, so a caller that
// holds the bytes need not go through storage.
//
// A document the trader asked to send but which cannot be read is an error
// rather than a silent omission: the certificate would otherwise claim to
// accompany a document that is not there.
func resolveAttachments(files Files, in *spscert.Input) error {
	var encoded int

	for i := range in.Certificate.Attachments {
		a := &in.Certificate.Attachments[i]

		if a.Key != "" && a.Base64 == "" {
			if files == nil {
				return fmt.Errorf("%s: this deployment has no document storage configured, so it cannot be attached", a.ID)
			}
			content, contentType, err := readFile(files, a.Key)
			if err != nil {
				return fmt.Errorf("%s (%s): %w", a.ID, a.Filename, err)
			}
			if len(content) > maxAttachmentBytes {
				return fmt.Errorf("%s (%s) is %s; the limit for a document sent with the certificate is %s",
					a.ID, a.Filename, humanSize(len(content)), humanSize(maxAttachmentBytes))
			}
			a.Base64 = base64.StdEncoding.EncodeToString(content)
			if a.MimeCode == "" {
				a.MimeCode = contentType
			}
		}
		a.Key = ""

		// Every attached file carries a type the Hub accepts, or it is refused
		// here. The validator rejects an empty mimeCode wherever a binary object
		// is present, and rejects the file outright for a type outside the four
		// the guidelines allow — either way as a validation fault naming nothing
		// the trader can act on.
		if a.Base64 != "" {
			mime := canonicalHubMime(a.Filename, a.MimeCode)
			if mime == "" {
				return fmt.Errorf("%s (%s) is not a JPG, GIF, PNG or PDF, which is all the Hub accepts with a certificate",
					a.ID, a.Filename)
			}
			a.MimeCode = mime
		}

		// Every document counts towards the certificate's own ceiling,
		// including one whose content the caller supplied ready-encoded.
		encoded += len(a.Base64)
		if encoded > maxCertificateBytes {
			return fmt.Errorf("the documents selected come to %s once encoded into the certificate, over the %s a certificate may be; send fewer of them",
				humanSize(encoded), humanSize(maxCertificateBytes))
		}
	}
	return nil
}

// readFile pulls one document out of storage, under this package's own
// deadline.
func readFile(files Files, key string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), attachmentReadTimeout)
	defer cancel()

	body, contentType, err := files.Download(ctx, key)
	if err != nil {
		return nil, "", fmt.Errorf("could not be read from storage: %w", err)
	}
	defer func() { _ = body.Close() }()

	// One byte over the limit is enough to refuse it, and stopping there keeps
	// a runaway file from being read into memory in full.
	content, err := io.ReadAll(io.LimitReader(body, maxAttachmentBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("could not be read from storage: %w", err)
	}
	return content, strings.TrimSpace(contentType), nil
}

// The types the Hub accepts for an attachment: "Format: JPG, GIF, PNG and PDF"
// (ePhyto Guidelines v2.12, ram:AttachmentBinaryObject). Anything else is
// refused on receipt, so it is refused here instead.
var hubMimes = map[string]string{
	"application/pdf": "application/pdf",
	"image/jpeg":      "image/jpeg",
	"image/jpg":       "image/jpeg", // seen from browsers; canonicalised
	"image/png":       "image/png",
	"image/gif":       "image/gif",
}

// extensionMimes maps the extensions those four types are stored under.
var extensionMimes = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
}

// canonicalHubMime returns the type this file should be sent as, or "" when it
// is not one the Hub accepts.
//
// The stored type is preferred when it names one of the four, and the extension
// is used when it does not — which is the ordinary case rather than the
// exceptional one. Storage answers "application/octet-stream" whenever it was
// given nothing better (core/storage/drivers), and the upload path sends that
// same default for any file the browser could not type, so a perfectly good PDF
// routinely arrives here described as bytes. Believing that would put a
// mimeCode on the certificate the validator refuses.
//
// Neither source placing the file is a refusal rather than a guess: the Hub
// would reject it anyway, and it is only here that the message can name the
// document and reach the trader.
func canonicalHubMime(filename, contentType string) string {
	// Storage may return parameters alongside the type ("text/plain;
	// charset=utf-8"), which are not part of the comparison.
	stored := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(stored, ";"); i != -1 {
		stored = strings.TrimSpace(stored[:i])
	}
	if mime, ok := hubMimes[stored]; ok {
		return mime
	}
	return extensionMimes[strings.ToLower(filepath.Ext(filename))]
}

// humanSize renders a byte count the way a trader would read it, in the unit
// the number calls for — the limits here span bytes to a hundred megabytes.
func humanSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d bytes", n)
	case n < 1024*1024:
		return fmt.Sprintf("%d KB", (n+1023)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
