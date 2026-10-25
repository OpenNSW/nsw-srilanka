// Package storage is the file storage this deployment keeps the files uploaded
// to it in: a storage backend of its own (core/storage — local disk or S3).
// Files other services own are opened through those services (see Files).
//
// Everything outside this package depends on the Service interface rather than
// on core/storage directly.
package storage

import (
	"context"
	"fmt"
	"io"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
)

// Service is the file storage the rest of the application uses. It is
// satisfied by core/storage's *Service.
type Service interface {
	// Upload reserves a key for a file and returns its metadata, including
	// the URL the client uploads the file to.
	Upload(ctx context.Context, filename string, size int64, mime string) (*corestorage.FileMetadata, error)
	// Download streams a stored file and returns its MIME type.
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
	// DownloadURL returns a time-limited URL the client downloads the file from,
	// the Unix time it expires at, the type the file was stored with and how the
	// URL serves it: inline when a browser opens the file, attachment when it
	// saves it.
	DownloadURL(ctx context.Context, key string) (corestorage.Download, error)
	// Save stores content this service produced itself — a generated
	// document, say — under a new key and returns its metadata. Unlike
	// Upload, nothing is left for a client to do, and the upload limits don't
	// apply. size is the length of body; it is recorded as given.
	Save(ctx context.Context, filename, mime string, body io.Reader, size int64) (*corestorage.FileMetadata, error)
}

// Stack is the storage service storage.type selected, and the routes that stand
// in for its backend when it is local disk.
type Stack struct {
	Service Service
	// LocalContent serves the upload/download content routes that stand in
	// for S3 when this deployment stores files on local disk. Nil otherwise:
	// with S3 the client talks to the bucket.
	LocalContent *corestorage.LocalContentHandler
}

// New builds the storage stack for the core/storage backend cfg.Type names.
func New(ctx context.Context, cfg Config) (*Stack, error) {
	if err := cfg.validateBackend(); err != nil {
		return nil, err
	}
	// The local content routes sit beside the rest of the storage API, not
	// wherever core/storage's default puts them. validateBackend allows only
	// an unset or matching value here, so nothing configured is overridden.
	cfg.Local.RoutePrefix = RoutePrefix
	driver, err := corestorage.NewStorageFromConfig(ctx, cfg.Config)
	if err != nil {
		return nil, fmt.Errorf("storage backend: %w", err)
	}
	svc := corestorage.NewService(driver,
		corestorage.WithAllowedUploadTypes(cfg.AllowedUploadTypes...),
		corestorage.WithInlineTypes(cfg.InlineTypes...),
		corestorage.WithMaxUploadSize(cfg.MaxUploadBytes),
	)

	stack := &Stack{Service: svc}
	if local, ok := driver.(*drivers.LocalFSDriver); ok {
		stack.LocalContent = corestorage.NewLocalContentHandler(local)
	}
	return stack, nil
}
