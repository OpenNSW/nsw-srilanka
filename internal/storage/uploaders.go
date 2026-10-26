package storage

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"path/filepath"
	"slices"
	"strings"

	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// Uploader reserves a key for a file and returns the URL to upload it to.
// Service satisfies it, and so does what limits the types of an upload.
type Uploader interface {
	Upload(ctx context.Context, filename string, size int64, mime string) (*corestorage.FileMetadata, error)
}

// typePolicy decides whether the caller in ctx may upload a file of the type
// the upload names, as sent.
type typePolicy func(ctx context.Context, mimeType string) bool

// limitedUploader refuses an upload whose type the policy does not allow, with
// corestorage.ErrContentTypeNotAllowed, before next sees it.
type limitedUploader struct {
	next   Uploader
	allows typePolicy
}

func limitTypes(next Uploader, allows typePolicy) Uploader {
	return limitedUploader{next: next, allows: allows}
}

func (u limitedUploader) Upload(ctx context.Context, filename string, size int64, mimeType string) (*corestorage.FileMetadata, error) {
	if !u.allows(ctx, mimeType) {
		return nil, fmt.Errorf("%w: %q", corestorage.ErrContentTypeNotAllowed, mimeType)
	}
	return u.next.Upload(ctx, filename, size, mimeType)
}

// ExtensionMismatchError refuses an upload whose file name ends in an extension
// its type does not have, such as "invoice.exe" sent as application/pdf: the
// stored file keeps the name's extension, and is downloaded under it.
type ExtensionMismatchError struct {
	// Extension is the file name's, with its dot, as sent.
	Extension string
	// MediaType is the upload's type, without parameters.
	MediaType string
}

func (e *ExtensionMismatchError) Error() string {
	return fmt.Sprintf("storage: file name extension %q does not match type %q", e.Extension, e.MediaType)
}

// extensionMatched makes an upload's file name agree with its type before next
// sees it: a name without an extension is given its type's first, and one with
// an extension its type does not have is refused with an
// *ExtensionMismatchError. A type with no known extensions is passed on
// unchecked; which types may be uploaded at all is the type lists' to decide.
type extensionMatched struct {
	next Uploader
}

func matchExtension(next Uploader) Uploader {
	return extensionMatched{next: next}
}

func (u extensionMatched) Upload(ctx context.Context, filename string, size int64, mimeType string) (*corestorage.FileMetadata, error) {
	if mt, ok := mediaType(mimeType); ok {
		if exts := extensionsOf(mt); len(exts) > 0 {
			// The extension core/storage keeps in the key it builds from
			// the name.
			switch ext := filepath.Ext(filename); {
			case ext == "":
				filename += exts[0]
			case !slices.Contains(exts, strings.ToLower(ext)):
				return nil, &ExtensionMismatchError{Extension: ext, MediaType: mt}
			}
		}
	}
	return u.next.Upload(ctx, filename, size, mimeType)
}

// extensionsOf returns the file extensions of mt, a media type, lower case with
// their dot, as Go's mime package knows them, plus .xml for application/xml,
// which its built-in table gives to text/xml only.
func extensionsOf(mt string) []string {
	exts, _ := mime.ExtensionsByType(mt)
	if mt == "application/xml" && !slices.Contains(exts, ".xml") {
		exts = append(exts, ".xml")
	}
	return exts
}

// anyCaller allows exactly the given types, whoever calls, comparing the whole
// string as core/storage does.
func anyCaller(types []string) typePolicy {
	set := typeSet(types)
	return func(_ context.Context, mimeType string) bool {
		_, ok := set[mimeType]
		return ok
	}
}

// machineClient allows base to every caller, and to a machine client the extra
// types granted to its client ID as well. It compares the media type alone,
// without parameters and case, so "text/html; charset=utf-8" is text/html.
func machineClient(base []string, extra map[string][]string) typePolicy {
	allowed := typeSet(base)
	extras := make(map[string]map[string]struct{}, len(extra))
	for clientID, types := range extra {
		extras[clientID] = typeSet(types)
	}
	return func(ctx context.Context, mimeType string) bool {
		mt, ok := mediaType(mimeType)
		if !ok {
			return false
		}
		if _, ok := allowed[mt]; ok {
			return true
		}
		p, ok := authn.FromContext(ctx)
		if !ok || p.ClientID == "" {
			return false
		}
		_, ok = extras[p.ClientID][mt]
		return ok
	}
}

// logged records the uploads an Uploader prepares for a machine client, and
// those it refuses or fails.
type logged struct{ next Uploader }

func (l logged) Upload(ctx context.Context, filename string, size int64, mimeType string) (*corestorage.FileMetadata, error) {
	meta, err := l.next.Upload(ctx, filename, size, mimeType)
	var clientID string
	if p, ok := authn.FromContext(ctx); ok {
		clientID = p.ClientID
	}
	if err != nil {
		slog.WarnContext(ctx, "storage: machine upload not prepared", "client_id", clientID, "mime_type", mimeType, "size", size, "error", err)
		return nil, err
	}
	slog.InfoContext(ctx, "storage: machine upload prepared", "client_id", clientID, "mime_type", mimeType, "size", size)
	return meta, nil
}

// newMachineUploads returns the Uploader behind the internal upload route:
// what users may upload (cfg.AllowedUploadTypes) plus the extra types each
// machine client is granted, by client ID.
//
// It prepares uploads with a core Service of its own over the same driver and
// without an upload allowlist, because core compares the whole MIME string with
// its list and refuses a type that is both uploadable and opened inline, while a
// generated document is sent as "text/html; charset=utf-8" and listed in
// inlineTypes. The limit is applied here, on the media type. How a stored file is
// served is still decided by the deployment's primary Service from inlineTypes.
func newMachineUploads(driver corestorage.StorageDriver, cfg Config, extra map[string][]string) Uploader {
	svc := corestorage.NewService(driver, corestorage.WithMaxUploadSize(cfg.MaxUploadBytes))
	return logged{limitTypes(svc, machineClient(cfg.AllowedUploadTypes, extra))}
}

// typeSet returns the set of the given media types.
func typeSet(types []string) map[string]struct{} {
	set := make(map[string]struct{}, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	return set
}

// mediaType returns the lower-case type/subtype of a Content-Type value, without
// its parameters. It reports false for a value that names no media type.
func mediaType(contentType string) (string, bool) {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil || mt == "" {
		return "", false
	}
	return mt, true
}
