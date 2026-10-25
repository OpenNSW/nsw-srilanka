package storage

import (
	"fmt"
	"strings"
	"time"

	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
)

// DefaultTokenTTLSeconds is how long a file token a user reads with lasts
// when storage.tokenTTLSeconds is unset: one hour.
const DefaultTokenTTLSeconds = 3600

// Config is the storage configuration. Type (storage.type) selects the
// core/storage backend this deployment stores its files in, and the embedded
// core/storage settings configure it. Those are inlined, so in config.yaml they
// sit beside the settings below under the same storage section.
type Config struct {
	corestorage.Config `yaml:",inline"`
	// AllowedUploadTypes are the MIME types clients may upload; any other
	// type is refused with 415. Required.
	AllowedUploadTypes []string `yaml:"allowedUploadTypes"`
	// InlineTypes are the media types a download opens in the browser; every
	// other file is saved. Required. A type a browser interprets as a document,
	// such as text/html, may be listed only when clients cannot upload it: it is
	// for what this service stores itself.
	InlineTypes []string `yaml:"inlineTypes"`
	// MaxUploadBytes is the largest file, in bytes, clients may upload.
	// Required.
	MaxUploadBytes int64 `yaml:"maxUploadBytes"`
	// TokenKeyset holds the keys the file tokens callers hold in place of
	// stored values are sealed with: comma-separated "kid:key" pairs, newest
	// first, each key 32 random bytes in base64. Required, and different for
	// every deployment. Keep a key for as long as the tokens it
	// sealed must open; those given to OGAs never expire.
	TokenKeyset string `yaml:"tokenKeyset"`
	// TokenTTLSeconds is how long a file token a user reads with lasts, in
	// seconds: the tokens a task's view gives out. Optional:
	// DefaultTokenTTLSeconds when unset. A user's upload tokens, which can only
	// be submitted, and machine clients' tokens never expire.
	TokenTTLSeconds int `yaml:"tokenTTLSeconds"`
}

// TokenTTL returns how long a file token a user reads with lasts.
func (c Config) TokenTTL() time.Duration {
	if c.TokenTTLSeconds == 0 {
		return DefaultTokenTTLSeconds * time.Second
	}
	return time.Duration(c.TokenTTLSeconds) * time.Second
}

// Validate checks the file token settings, and then the core/storage backend
// Type selects. The local content routes always sit under RoutePrefix, so a
// storage.local.routePrefix that says otherwise is an error rather than
// silently ignored.
func (c Config) Validate() error {
	if err := c.validateTokens(); err != nil {
		return err
	}
	if err := c.validateBackend(); err != nil {
		return err
	}
	return c.Config.Validate()
}

func (c Config) validateTokens() error {
	if strings.TrimSpace(c.TokenKeyset) == "" {
		return fmt.Errorf("storage.tokenKeyset is required: comma-separated kid:key pairs, each key 32 random bytes in base64")
	}
	if _, err := filetoken.ParseKeyset(c.TokenKeyset); err != nil {
		return fmt.Errorf("storage.tokenKeyset: %w", err)
	}
	if c.TokenTTLSeconds < 0 {
		return fmt.Errorf("storage.tokenTTLSeconds must be greater than zero, or unset for %d, got %d", DefaultTokenTTLSeconds, c.TokenTTLSeconds)
	}
	return nil
}

// validateBackend checks what this application adds to a core/storage
// backend's settings. New runs it as well, so a configuration that never went
// through Validate is refused the same way: a conflicting route prefix is not
// silently replaced, and a non-positive size cannot reach core's panic.
func (c Config) validateBackend() error {
	if strings.TrimSpace(c.Type) == "proxy" {
		return fmt.Errorf(`storage.type "proxy" is not supported: this deployment stores the files uploaded to it itself, so set it to local or s3`)
	}
	if p := c.Local.RoutePrefix; p != "" && p != RoutePrefix {
		return fmt.Errorf("storage.local.routePrefix must be %q, where this application mounts its storage routes, or left unset; got %q", RoutePrefix, p)
	}
	if len(c.AllowedUploadTypes) == 0 {
		return fmt.Errorf("storage.allowedUploadTypes must list at least one MIME type")
	}
	if len(c.InlineTypes) == 0 {
		return fmt.Errorf("storage.inlineTypes must list at least one MIME type")
	}
	// core/storage compares types to both lists exactly, so each must be a
	// bare, lower-case type/subtype: no parameters, no wildcard.
	for _, t := range c.AllowedUploadTypes {
		if err := corestorage.ValidMediaType(t); err != nil {
			return fmt.Errorf("storage.allowedUploadTypes: %w", err)
		}
	}
	for _, t := range c.InlineTypes {
		if err := corestorage.ValidMediaType(t); err != nil {
			return fmt.Errorf("storage.inlineTypes: %w", err)
		}
	}
	if err := corestorage.CheckUploadTypes(c.AllowedUploadTypes, c.InlineTypes); err != nil {
		return fmt.Errorf("storage.allowedUploadTypes and storage.inlineTypes: %w", err)
	}
	if c.MaxUploadBytes <= 0 {
		return fmt.Errorf("storage.maxUploadBytes must be greater than zero, got %d", c.MaxUploadBytes)
	}
	return nil
}
