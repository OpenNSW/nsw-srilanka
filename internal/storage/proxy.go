package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OpenNSW/core/remote"
	corestorage "github.com/OpenNSW/core/storage"
)

// proxyFetchTimeout bounds fetching a presigned download URL, including
// streaming its body.
const proxyFetchTimeout = 60 * time.Second

// maxProxyErrorBody caps how much of a failed download's body is read into
// an error message.
const maxProxyErrorBody = 4 << 10

// ServiceCaller calls a service in the outbound services registry;
// *remote.Manager satisfies it.
type ServiceCaller interface {
	GetClient(id string) (*remote.Client, error)
	Call(ctx context.Context, serviceID string, req remote.Request, response any) error
}

// ProxyService is a Service that holds no files: every operation is
// forwarded to the storage API of the service that owns them, which also
// allocates the keys. The key it returns from Upload is the owning service's,
// and is the one to persist.
type ProxyService struct {
	caller ServiceCaller
	cfg    ProxyConfig
	// fetchClient fetches presigned download URLs. They point at the owning
	// service's storage backend rather than its API, and must be requested
	// without its API credentials — S3 rejects a presigned request that also
	// carries an Authorization header.
	fetchClient *http.Client
}

// NewProxyService creates a ProxyService. It fails if cfg's service is not in
// the registry, so a misnamed service is caught at startup rather than on the
// first upload.
func NewProxyService(caller ServiceCaller, cfg ProxyConfig) (*ProxyService, error) {
	if caller == nil {
		return nil, fmt.Errorf("storage proxy: a service caller is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("storage proxy: %w", err)
	}
	if _, err := caller.GetClient(cfg.Service); err != nil {
		return nil, fmt.Errorf("storage proxy: %w", err)
	}
	return &ProxyService{
		caller:      caller,
		cfg:         cfg,
		fetchClient: &http.Client{Timeout: proxyFetchTimeout},
	}, nil
}

// Upload asks the owning service to allocate a key for the file and presign
// an upload URL for it.
func (s *ProxyService) Upload(ctx context.Context, filename string, size int64, mime string) (*corestorage.FileMetadata, error) {
	var resp struct {
		corestorage.FileMetadata
		Error string `json:"error"`
	}
	err := s.caller.Call(ctx, s.cfg.Service, remote.Request{
		Method: http.MethodPost,
		Path:   s.cfg.UploadPath,
		Body: remote.JSONBody{V: map[string]any{
			"filename":  filename,
			"mime_type": mime,
			"size":      size,
		}},
	}, &resp)
	if err != nil {
		return nil, remoteFailure("upload", err, resp.Error)
	}
	if resp.Key == "" || resp.UploadURL == "" {
		return nil, fmt.Errorf("storage proxy: upload response missing key or upload_url")
	}
	if err := requireAbsoluteURL(resp.UploadURL); err != nil {
		return nil, err
	}
	meta := resp.FileMetadata
	return &meta, nil
}

// GetDownloadURL returns the owning service's time-limited download URL.
func (s *ProxyService) GetDownloadURL(ctx context.Context, key string) (string, error) {
	downloadURL, _, err := s.DownloadURL(ctx, key)
	return downloadURL, err
}

// DownloadURL returns the owning service's time-limited download URL together
// with the Unix time it expires at, as the owning service reported it.
func (s *ProxyService) DownloadURL(ctx context.Context, key string) (string, int64, error) {
	var resp struct {
		DownloadURL string `json:"download_url"`
		ExpiresAt   int64  `json:"expires_at"`
		Error       string `json:"error"`
	}
	err := s.caller.Call(ctx, s.cfg.Service, remote.Request{
		Method: http.MethodGet,
		Path:   keyPath(s.cfg.DownloadPath, key),
	}, &resp)
	if err != nil {
		return "", 0, remoteFailure("get download URL", err, resp.Error)
	}
	if resp.DownloadURL == "" {
		return "", 0, fmt.Errorf("storage proxy: download response missing download_url")
	}
	if err := requireAbsoluteURL(resp.DownloadURL); err != nil {
		return "", 0, err
	}
	return resp.DownloadURL, resp.ExpiresAt, nil
}

// Download streams the file from the owning service via its download URL.
func (s *ProxyService) Download(ctx context.Context, key string) (io.ReadCloser, string, error) {
	downloadURL, err := s.GetDownloadURL(ctx, key)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("storage proxy: build download request: %w", err)
	}
	resp, err := s.fetchClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("storage proxy: download %s: %w", key, err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxProxyErrorBody))
		return nil, "", fmt.Errorf("storage proxy: download %s: owning service returned %d: %s", key, resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return resp.Body, contentType, nil
}

// Delete removes the file from the owning service.
func (s *ProxyService) Delete(ctx context.Context, key string) error {
	var resp struct {
		Error string `json:"error"`
	}
	err := s.caller.Call(ctx, s.cfg.Service, remote.Request{
		Method: http.MethodDelete,
		Path:   keyPath(s.cfg.DeletePath, key),
	}, &resp)
	if err != nil {
		return remoteFailure("delete", err, resp.Error)
	}
	return nil
}

// RemoteRejection is a client error (4xx) the owning service returned for a
// request, other than a rejection of this service's own credentials. It is
// the caller's request that was refused — an unsupported file type, say — so
// ProxyHandler relays it rather than reporting a gateway failure.
type RemoteRejection struct {
	StatusCode int
	// Message is the owning service's error message, when it sent one.
	Message string
	err     error
}

func (e *RemoteRejection) Error() string {
	return e.err.Error()
}

func (e *RemoteRejection) Unwrap() error {
	return e.err
}

// remoteFailure wraps an error from the owning service's API, classifying a
// 4xx rejection of the request as a *RemoteRejection. remote reads the error
// body into the response before failing, which is where message comes from.
func remoteFailure(op string, err error, message string) error {
	wrapped := fmt.Errorf("storage proxy: %s: %w", op, err)

	var re *remote.RemoteError
	if !errors.As(err, &re) || re.StatusCode < 400 || re.StatusCode > 499 {
		return wrapped
	}
	// 401/403 mean the owning service refused this service's credentials —
	// a misconfiguration here, not a problem with the caller's request.
	if re.StatusCode == http.StatusUnauthorized || re.StatusCode == http.StatusForbidden {
		return wrapped
	}
	return &RemoteRejection{StatusCode: re.StatusCode, Message: message, err: wrapped}
}

// keyPath substitutes the escaped key for every {key} in a path template.
func keyPath(template, key string) string {
	return strings.ReplaceAll(template, KeyPlaceholder, url.PathEscape(key))
}

// requireAbsoluteURL rejects a relative URL from the owning service: handed to
// a client, it would resolve against this service instead of the owner.
func requireAbsoluteURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("storage proxy: owning service returned non-absolute URL %q; its storage public URL must be absolute", raw)
	}
	return nil
}
