package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/google/uuid"
)

// Writer stores a file the backend itself holds the bytes of, rather than one
// a client uploads. It is how a document another service issued — a payment
// slip, a gate pass — comes to be kept here instead of linked to there.
//
// The key it returns is the one to persist and to hand the trader: the same
// uuid-plus-extension shape Upload mints, which is what the portal recognises
// as a stored file rather than an outside link.
type Writer interface {
	Store(ctx context.Context, filename string, content []byte, mime string) (string, error)
}

// backendWriter writes straight through the storage driver. core/storage's
// Service only presigns uploads for a client to PUT to; its driver already
// accepts bytes, so the server-side write needs no round trip through a URL.
type backendWriter struct {
	driver corestorage.StorageDriver
}

func (w backendWriter) Store(ctx context.Context, filename string, content []byte, mime string) (string, error) {
	key := uuid.NewString() + filepath.Ext(filename)
	if err := w.driver.Save(ctx, key, bytes.NewReader(content), mime); err != nil {
		return "", fmt.Errorf("storage: save %s: %w", filename, err)
	}
	return key, nil
}

// Store keeps content in the owning service's storage. That service allocates
// keys and only accepts bytes at the URL it presigns, so this does what a
// client would: reserve the key, then PUT the file to the upload URL.
//
// The PUT goes out without this service's API credentials, for the same reason
// Download fetches without them: the URL points at the storage backend, and S3
// rejects a presigned request that also carries an Authorization header.
func (s *ProxyService) Store(ctx context.Context, filename string, content []byte, mime string) (string, error) {
	meta, err := s.Upload(ctx, filename, int64(len(content)), mime)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, meta.UploadURL, bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("storage proxy: build upload request: %w", err)
	}
	req.Header.Set("Content-Type", mime)
	resp, err := s.fetchClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage proxy: upload %s: %w", filename, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxProxyErrorBody))
		return "", fmt.Errorf("storage proxy: upload %s: owning service returned %d: %s", filename, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return meta.Key, nil
}
