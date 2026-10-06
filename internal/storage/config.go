package storage

import (
	"fmt"
	"mime"
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
	// AllowedUploadTypes are the MIME types clients may upload; any other
	// type is refused with 415. Required for the local and s3 backends; not
	// read in proxy mode, where the owning service applies its own.
	AllowedUploadTypes []string `yaml:"allowedUploadTypes"`
	// MaxUploadBytes is the largest file, in bytes, clients may upload.
	// Required for the local and s3 backends; not read in proxy mode.
	MaxUploadBytes int64 `yaml:"maxUploadBytes"`
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
// an unknown backend type. The local content routes always sit under
// RoutePrefix, so a storage.local.routePrefix that says otherwise is an error
// rather than silently ignored.
func (c Config) Validate() error {
	if c.IsProxy() {
		return c.Proxy.Validate()
	}
	if err := c.validateBackend(); err != nil {
		return err
	}
	return c.Config.Validate()
}

// validateBackend checks what this application adds to a core/storage
// backend's settings. New runs it as well, so a configuration that never went
// through Validate is refused the same way: a conflicting route prefix is not
// silently replaced, and a non-positive size cannot reach core's panic.
func (c Config) validateBackend() error {
	if p := c.Local.RoutePrefix; p != "" && p != RoutePrefix {
		return fmt.Errorf("storage.local.routePrefix must be %q, where this application mounts its storage routes, or left unset; got %q", RoutePrefix, p)
	}
	if len(c.AllowedUploadTypes) == 0 {
		return fmt.Errorf("storage.allowedUploadTypes must list at least one MIME type")
	}
	for _, t := range c.AllowedUploadTypes {
		// core/storage compares an upload's type to these exactly, so each
		// must be a bare, lower-case type/subtype: no parameters, no wildcard.
		mediaType, params, err := mime.ParseMediaType(t)
		if err != nil || len(params) > 0 || mediaType != t || !strings.Contains(t, "/") || strings.Contains(t, "*") {
			return fmt.Errorf("storage.allowedUploadTypes: %q is not a bare MIME type such as \"application/pdf\"", t)
		}
	}
	if c.MaxUploadBytes <= 0 {
		return fmt.Errorf("storage.maxUploadBytes must be greater than zero, got %d", c.MaxUploadBytes)
	}
	return nil
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
