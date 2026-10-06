// Package storage selects the file storage this deployment serves uploads
// from: either a storage backend of its own (core/storage — local disk or S3)
// or a proxy onto another service that owns the files.
//
// Everything outside this package depends on the Service and Handler
// interfaces rather than on core/storage directly, so the choice is made once,
// here, from storage.type.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
)

// Service is the file storage the rest of the application uses. It is
// satisfied by core/storage's *Service and by ProxyService.
type Service interface {
	// Upload reserves a key for a file and returns its metadata, including
	// the URL the client uploads the file to.
	Upload(ctx context.Context, filename string, size int64, mime string) (*corestorage.FileMetadata, error)
	// Download streams a stored file and returns its MIME type.
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
	// GetDownloadURL returns a time-limited URL the client downloads the file from.
	GetDownloadURL(ctx context.Context, key string) (string, error)
	// Delete removes a stored file.
	Delete(ctx context.Context, key string) error
}

// Handler serves the storage API routes. It is satisfied by core/storage's
// *HTTPHandler and by ProxyHandler.
type Handler interface {
	Upload(w http.ResponseWriter, r *http.Request)
	Download(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
}

// UploadTypes are the MIME types this deployment's own storage accepts
// uploads of; any other type is rejected with 415. core/storage accepts any
// type unless told otherwise, so the list is this application's policy. It is
// the list core/storage enforced itself up to v0.2.0.
var UploadTypes = []string{
	"application/pdf",
	"image/jpeg",
	"image/png",
	"image/gif",
	"image/webp",
	// .xlsx only: the OOXML spreadsheet format cannot carry VBA macros
	// (macro-enabled workbooks use .xlsm), unlike legacy .xls which is a
	// known malware vector and stays prohibited.
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// Stack is the storage service and HTTP handlers storage.type selected.
type Stack struct {
	Service Service
	Handler Handler
	// LocalContent serves the upload/download content routes that stand in
	// for S3 when this deployment stores files on local disk. Nil otherwise:
	// with S3 the client talks to the bucket, and behind a proxy it talks to
	// the owning service.
	LocalContent *corestorage.LocalContentHandler
}

// New builds the storage stack for cfg.Type: a proxy onto another service
// when it is TypeProxy, otherwise the core/storage backend it names. caller
// reaches the owning service and is only used in proxy mode.
func New(ctx context.Context, cfg Config, caller ServiceCaller) (*Stack, error) {
	if cfg.IsProxy() {
		svc, err := NewProxyService(caller, cfg.Proxy)
		if err != nil {
			return nil, err
		}
		return &Stack{Service: svc, Handler: NewProxyHandler(svc)}, nil
	}

	driver, err := corestorage.NewStorageFromConfig(ctx, cfg.Config)
	if err != nil {
		return nil, fmt.Errorf("storage backend: %w", err)
	}
	svc := corestorage.NewService(driver, corestorage.WithAllowedUploadTypes(UploadTypes...))

	stack := &Stack{Service: svc, Handler: corestorage.NewHTTPHandler(svc)}
	if local, ok := driver.(*drivers.LocalFSDriver); ok {
		stack.LocalContent = corestorage.NewLocalContentHandler(local)
	}
	return stack, nil
}
