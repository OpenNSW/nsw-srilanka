package storage

import (
	"fmt"
	"strings"

	corestorage "github.com/OpenNSW/core/storage"
)

// TypeProxy is the storage type (storage.type) that serves storage from
// another service instead of a backend of this deployment's own. Every other
// type is a core/storage backend type.
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

// Config is the storage configuration. Type (storage.type) selects either
// TypeProxy, which uses Proxy, or a core/storage backend, which uses the
// embedded core/storage settings. Those are inlined, so in config.yaml they sit
// beside proxy under the same storage section.
type Config struct {
	corestorage.Config `yaml:",inline"`
	// Proxy is used when Type is TypeProxy: files are served from another
	// service that owns them.
	Proxy ProxyConfig `yaml:"proxy"`
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

// ProxyConfig configures proxy mode (storage.type: proxy): which service owns
// the files and where it serves its storage API.
type ProxyConfig struct {
	// Service is the owning service's ID in the outbound services registry
	// (services.json), which supplies its URL, authentication and timeout.
	Service string `yaml:"service"`
	// UploadPath is the owning service's upload endpoint (POST), which
	// allocates a key and returns the file metadata with an upload URL.
	UploadPath string `yaml:"uploadPath"`
	// DownloadPath is its download endpoint (GET), which returns a
	// download URL. Must contain {key}.
	DownloadPath string `yaml:"downloadPath"`
	// DeletePath is its delete endpoint (DELETE). Must contain {key}.
	DeletePath string `yaml:"deletePath"`
}

// Validate reports whether the proxy configuration is usable.
func (c ProxyConfig) Validate() error {
	if strings.TrimSpace(c.Service) == "" {
		return fmt.Errorf("storage.proxy.service is required when storage.type is %s", TypeProxy)
	}
	for name, path := range map[string]string{
		"storage.proxy.uploadPath":   c.UploadPath,
		"storage.proxy.downloadPath": c.DownloadPath,
		"storage.proxy.deletePath":   c.DeletePath,
	} {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s is required when storage.type is %s", name, TypeProxy)
		}
	}
	if !strings.Contains(c.DownloadPath, KeyPlaceholder) {
		return fmt.Errorf("storage.proxy.downloadPath must contain %s", KeyPlaceholder)
	}
	if !strings.Contains(c.DeletePath, KeyPlaceholder) {
		return fmt.Errorf("storage.proxy.deletePath must contain %s", KeyPlaceholder)
	}
	return nil
}
