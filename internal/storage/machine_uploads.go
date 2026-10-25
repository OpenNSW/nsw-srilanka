package storage

import (
	"context"
	"fmt"
	"log/slog"
	"mime"

	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// Uploader reserves a key for a file and returns the URL to upload it to. Both
// Service and MachineUploads satisfy it.
type Uploader interface {
	Upload(ctx context.Context, filename string, size int64, mime string) (*corestorage.FileMetadata, error)
}

// MachineUploads is the Uploader behind the internal upload route, for machine
// clients. It accepts the types users may upload, plus the extra types
// storage.allowedInternalUploadTypes grants the calling client.
//
// Its core Service has no upload allowlist: core compares the whole MIME string
// with its list and refuses a type that is both uploadable and opened inline,
// while a generated document is sent as "text/html; charset=utf-8" and listed
// in inlineTypes. The check is made here instead, on the media type alone, and
// the type as sent is what the upload is bound to and stored with.
//
// It only prepares uploads. How a stored file is served is decided by the
// deployment's primary Service from inlineTypes.
type MachineUploads struct {
	svc *corestorage.Service
	// base is what every caller may upload.
	base map[string]struct{}
	// extra is what a client may upload beyond base, by client ID.
	extra map[string]map[string]struct{}
}

func newMachineUploads(driver corestorage.StorageDriver, cfg Config, extra map[string][]string) *MachineUploads {
	extras := make(map[string]map[string]struct{}, len(extra))
	for clientID, types := range extra {
		extras[clientID] = typeSet(types)
	}
	return &MachineUploads{
		svc:   corestorage.NewService(driver, corestorage.WithMaxUploadSize(cfg.MaxUploadBytes)),
		base:  typeSet(cfg.AllowedUploadTypes),
		extra: extras,
	}
}

// Service is the core Service uploads are prepared with, for wiring that
// applies to every core Service of the deployment, such as its auditor.
func (m *MachineUploads) Service() *corestorage.Service {
	return m.svc
}

// Upload refuses a type the calling client may not upload with
// corestorage.ErrContentTypeNotAllowed, and otherwise prepares the upload.
func (m *MachineUploads) Upload(ctx context.Context, filename string, size int64, mimeType string) (*corestorage.FileMetadata, error) {
	var clientID string
	if p, ok := authn.FromContext(ctx); ok {
		clientID = p.ClientID
	}
	mt, ok := mediaType(mimeType)
	if !ok || !m.allows(clientID, mt) {
		return nil, fmt.Errorf("%w: %q", corestorage.ErrContentTypeNotAllowed, mimeType)
	}
	meta, err := m.svc.Upload(ctx, filename, size, mimeType)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "storage: machine upload prepared", "client_id", clientID, "media_type", mt, "size", size)
	return meta, nil
}

func (m *MachineUploads) allows(clientID, mediaType string) bool {
	if _, ok := m.base[mediaType]; ok {
		return true
	}
	if clientID == "" {
		return false
	}
	_, ok := m.extra[clientID][mediaType]
	return ok
}

// typeSet returns the set of the given media types.
func typeSet(types []string) map[string]struct{} {
	set := make(map[string]struct{}, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	return set
}

// inTypeSet reports whether the media type of contentType is in set.
func inTypeSet(set map[string]struct{}, contentType string) bool {
	mt, ok := mediaType(contentType)
	if !ok {
		return false
	}
	_, in := set[mt]
	return in
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
