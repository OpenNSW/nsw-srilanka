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

		// The Hub's validator rejects an empty mimeCode wherever a binary
		// object is present, so every attached file carries one.
		if a.Base64 != "" && a.MimeCode == "" {
			a.MimeCode = mimeCodeFor(a.Filename)
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

// mimeCodeFor maps a filename extension to the MIME type the Hub expects in
// the mimeCode attribute. An unknown extension gets the generic binary type
// rather than an empty attribute, which the validator rejects.
func mimeCodeFor(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".txt":
		return "text/plain"
	case ".xml":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
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
