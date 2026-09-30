package storage

import (
	"fmt"
	"strings"

	corestorage "github.com/OpenNSW/core/storage"
)

// TypeProxy is the STORAGE_TYPE that serves storage from another service
// instead of a backend of this deployment's own. Every other STORAGE_TYPE is
// a core/storage backend type.
const TypeProxy = "proxy"

// KeyPlaceholder marks where the storage key goes in ProxyConfig's
// DownloadPath and DeletePath.
const KeyPlaceholder = "{key}"

// Default ProxyConfig endpoint paths: the storage routes this application
// itself mounts, so a proxy onto another deployment of it needs none set.
const (
	DefaultProxyUploadPath   = "/api/v1/storage"
	DefaultProxyDownloadPath = "/api/v1/storage/" + KeyPlaceholder
	DefaultProxyDeletePath   = "/api/v1/storage/" + KeyPlaceholder
)

// Config is the storage configuration. Type (STORAGE_TYPE) selects either
// TypeProxy, which uses Proxy, or a core/storage backend, which uses the
// embedded core/storage settings.
type Config struct {
	corestorage.Config
	// Proxy is used when Type is TypeProxy: files are served from another
	// service that owns them.
	Proxy ProxyConfig
}

// IsProxy reports whether Type selects proxy mode.
func (c Config) IsProxy() bool {
	return strings.TrimSpace(c.Type) == TypeProxy
}

// Validate checks the configuration Type selects: the proxy settings in proxy
// mode, otherwise the core/storage backend's — which would reject "proxy" as
// an unknown backend type.
func (c Config) Validate() error {
	if c.IsProxy() {
		return c.Proxy.Validate()
	}
	return c.Config.Validate()
}

// ProxyConfig configures proxy mode (STORAGE_TYPE=proxy): which service owns
// the files and where it serves its storage API.
type ProxyConfig struct {
	// Service is the owning service's ID in the outbound services registry
	// (services.json), which supplies its URL, authentication and timeout.
	Service string
	// UploadPath is the owning service's upload endpoint (POST), which
	// allocates a key and returns the file metadata with an upload URL.
	UploadPath string
	// DownloadPath is its download endpoint (GET), which returns a
	// download URL. Must contain {key}.
	DownloadPath string
	// DeletePath is its delete endpoint (DELETE). Must contain {key}.
	DeletePath string
}

// Validate reports whether the proxy configuration is usable.
func (c ProxyConfig) Validate() error {
	if strings.TrimSpace(c.Service) == "" {
		return fmt.Errorf("STORAGE_PROXY_SERVICE is required when STORAGE_TYPE=%s", TypeProxy)
	}
	for name, path := range map[string]string{
		"STORAGE_PROXY_UPLOAD_PATH":   c.UploadPath,
		"STORAGE_PROXY_DOWNLOAD_PATH": c.DownloadPath,
		"STORAGE_PROXY_DELETE_PATH":   c.DeletePath,
	} {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s is required when STORAGE_TYPE=%s", name, TypeProxy)
		}
	}
	if !strings.Contains(c.DownloadPath, KeyPlaceholder) {
		return fmt.Errorf("STORAGE_PROXY_DOWNLOAD_PATH must contain %s", KeyPlaceholder)
	}
	if !strings.Contains(c.DeletePath, KeyPlaceholder) {
		return fmt.Errorf("STORAGE_PROXY_DELETE_PATH must contain %s", KeyPlaceholder)
	}
	return nil
}
