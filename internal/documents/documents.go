// Package documents keeps a copy of documents other services issue — a payment
// slip, a receipt, a gate pass — in this deployment's own storage, so the
// trader is handed our link rather than theirs.
//
// A provider's link is usually a signed URL on its own host: it expires on a
// schedule we do not control, it works only while that host is up, and it
// leaves us holding no record of what the trader was shown. Fetching the
// document when it is issued and storing it fixes all three. The storage key
// that replaces the link is what the portal already resolves into a download.
package documents

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/OpenNSW/core/remote"
)

// Store keeps a document's bytes and returns the storage key it is kept
// under. The storage package's Writer satisfies it.
type Store interface {
	Store(ctx context.Context, filename string, content []byte, mime string) (string, error)
}

// Fetcher retrieves a document through the service that issued it.
// *remote.Manager satisfies it.
//
// Going through the service rather than a bare HTTP client is the point: the
// remote client refuses an absolute URL whose scheme and host differ from the
// service's configured one, so a link in a provider's answer can only ever
// reach that provider — never an internal address it was crafted to name.
type Fetcher interface {
	CallRaw(ctx context.Context, serviceID string, req remote.Request) (*remote.RawResponse, error)
}

// Field names one captured value that holds a document link, and the base
// name the stored copy is filed under.
type Field struct {
	Key  string
	Name string
}

// fetchTimeout bounds one document fetch, retries included. The document is
// fetched on the path of the step that received the link, so a slow provider
// must not hold that step for long; when it runs out the link is kept as it
// was and the step moves on.
const fetchTimeout = 20 * time.Second

// fetchRetry retries the transient failures a document host has been seen to
// produce. A 4xx is not retried: a refused or expired signature does not
// recover by asking again.
var fetchRetry = remote.RetryConfig{
	MaxRetries:      2,
	InitialBackoff:  500 * time.Millisecond,
	MaxBackoff:      2 * time.Second,
	RetryableStatus: []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
}

// storageKey matches a key this deployment's storage minted — what the portal
// treats as a stored file. A value already in this shape has been archived and
// is left alone, which keeps archiving safe to run twice on the same answer.
var storageKey = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(\.[a-zA-Z0-9]+)?$`)

// extensions are the file types a provider's documents arrive as, by MIME
// type. The extension is part of the key, and the portal needs one to know
// the key for a file.
var extensions = map[string]string{
	"application/pdf": ".pdf",
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/gif":       ".gif",
	"image/svg+xml":   ".svg",
	"image/webp":      ".webp",
}

// Archiver fetches documents and keeps them in storage.
type Archiver struct {
	store Store
	fetch Fetcher
}

// NewArchiver binds an archiver to the storage it writes to and the services
// it fetches through.
func NewArchiver(store Store, fetch Fetcher) *Archiver {
	return &Archiver{store: store, fetch: fetch}
}

// Archive stores the document src points at and returns its storage key.
//
// src is either a link on serviceID's host or a data: URI — the shape a
// provider sends a small image in, such as a barcode, inline in its answer.
// name is the base filename the copy is filed under; the extension comes from
// what the document turns out to be.
func (a *Archiver) Archive(ctx context.Context, serviceID, src, name string) (string, error) {
	var (
		content  []byte
		mimeType string
		err      error
	)
	if strings.HasPrefix(src, "data:") {
		content, mimeType, err = decodeDataURI(src)
	} else {
		content, mimeType, err = a.download(ctx, serviceID, src)
	}
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", errors.New("documents: the document is empty")
	}

	ext, ok := extensions[mimeType]
	if !ok {
		ext = path.Ext(urlPath(src))
	}
	if ext == "" {
		ext = ".bin"
	}
	return a.store.Store(ctx, name+ext, content, mimeType)
}

// ArchiveFields replaces each named link in out with the storage key of the
// document it points at.
//
// A document that cannot be fetched or stored keeps its original link, and
// the failure is logged rather than returned. The step that received the link
// has already succeeded — the invoice is issued, the pass is granted — and a
// trader shown the provider's link is no worse off than before this existed,
// where one shown nothing has lost the document outright.
func (a *Archiver) ArchiveFields(ctx context.Context, serviceID string, out map[string]any, fields []Field) {
	for _, f := range fields {
		src, _ := out[f.Key].(string)
		src = strings.TrimSpace(src)
		if src == "" || storageKey.MatchString(src) {
			continue
		}
		key, err := a.Archive(ctx, serviceID, src, f.Name)
		if err != nil {
			slog.WarnContext(ctx, "documents: kept the provider's link; the document could not be stored",
				"service_id", serviceID, "field", f.Key, "error", err)
			continue
		}
		out[f.Key] = key
	}
}

// download fetches a document through serviceID.
func (a *Archiver) download(ctx context.Context, serviceID, src string) ([]byte, string, error) {
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, "", fmt.Errorf("documents: %q is not a link this can fetch", redact(src))
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	retry := fetchRetry
	resp, err := a.fetch.CallRaw(ctx, serviceID, remote.Request{
		Method: http.MethodGet,
		Path:   src,
		Retry:  &retry,
	})
	if err != nil {
		// A transport error quotes the URL it failed on, signature and all.
		return nil, "", fmt.Errorf("documents: fetch %s: %s", redact(src), strings.ReplaceAll(err.Error(), src, redact(src)))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("documents: fetch %s: %s answered %d", redact(src), serviceID, resp.StatusCode)
	}
	mimeType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(resp.Body)
		mimeType, _, _ = mime.ParseMediaType(mimeType)
	}
	if mimeType == "text/html" {
		// A login page or an error page served with 200 is not the document,
		// and storing it would put a broken file behind the trader's link.
		return nil, "", fmt.Errorf("documents: fetch %s: %s answered a web page, not a document", redact(src), serviceID)
	}
	return resp.Body, mimeType, nil
}

// decodeDataURI reads a base64 data: URI. Only base64 is accepted: it is the
// form binary images arrive in, and a percent-encoded one has not been seen.
func decodeDataURI(src string) ([]byte, string, error) {
	header, data, ok := strings.Cut(strings.TrimPrefix(src, "data:"), ",")
	if !ok || !strings.HasSuffix(header, ";base64") {
		return nil, "", errors.New("documents: a data: URI must be base64-encoded")
	}
	mimeType, _, _ := mime.ParseMediaType(strings.TrimSuffix(header, ";base64"))
	content, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", fmt.Errorf("documents: decode data: URI: %w", err)
	}
	return content, mimeType, nil
}

// urlPath is src's path, for an extension when the document did not say what
// it is.
func urlPath(src string) string {
	if strings.HasPrefix(src, "data:") {
		return ""
	}
	u, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return u.Path
}

// redact drops a link's query string before it is logged: on a signed URL the
// query is the credential.
func redact(src string) string {
	if strings.HasPrefix(src, "data:") {
		return "data: URI"
	}
	base, _, _ := strings.Cut(src, "?")
	return base
}
