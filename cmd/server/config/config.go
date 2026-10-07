package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/refid"
	"github.com/OpenNSW/core/temporal"

	"github.com/LSFLK/argus/pkg/audit"

	integrations "github.com/OpenNSW/nsw-srilanka/external-integration"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	nswdatabase "github.com/OpenNSW/nsw-srilanka/internal/database"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// defaultConfigPath is where Load looks for config.yaml when CONFIG_PATH is
// unset.
const defaultConfigPath = "configs/config.yaml"

// Config holds all configuration for the application, in the shape of
// config.yaml: each field is one top-level section of the file.
type Config struct {
	// Mode is what this deployment runs as: TNSW or an agency. Required.
	Mode Mode `yaml:"mode"`

	Database     database.Config     `yaml:"db"`
	Server       ServerConfig        `yaml:"server"`
	CORS         cors.Config         `yaml:"cors"`
	Storage      nswstorage.Config   `yaml:"storage"`
	Authn        authn.Config        `yaml:"authn"`
	Notification NotificationConfig  `yaml:"notification"`
	Temporal     temporal.Config     `yaml:"temporal"`
	Audit        AuditConfig         `yaml:"audit"`
	Integrations integrations.Config `yaml:"integrations"`

	ArtifactLoader loaders.Config `yaml:"artifactLoader"`

	// RefID holds the reference ID formats the REFID_GENERATOR task plugin
	// generates from. Optional: with no issuers, generation is disabled and a
	// template using the plugin fails at run time (see bootstrap.initRefIDs).
	RefID refid.Config `yaml:"refid"`
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	Port                     int           `yaml:"port"`
	ServiceURL               string        `yaml:"serviceURL"`
	ServicesConfigPath       string        `yaml:"servicesConfigPath"`
	PaymentMethodsConfigPath string        `yaml:"paymentMethodsConfigPath"`
	CatalogConfigPath        string        `yaml:"catalogConfigPath"`
	LogLevel                 slog.Level    `yaml:"logLevel"`
	MaxRequestBytes          int64         `yaml:"maxRequestBytes"`
	ReadHeaderTimeout        time.Duration `yaml:"readHeaderTimeout"`
	ReadTimeout              time.Duration `yaml:"readTimeout"`
	WriteTimeout             time.Duration `yaml:"writeTimeout"`
	IdleTimeout              time.Duration `yaml:"idleTimeout"`
}

// AuditConfig is what a deployment sets for the Argus audit client. The
// client's own audit.Config is an SDK constructor input — it carries a signer
// func and tuning knobs, and no yaml tags — so it is built from this rather
// than decoded into.
type AuditConfig struct {
	BaseURL string `yaml:"baseURL"`
	APIKey  string `yaml:"apiKey"`
}

// ClientConfig is the audit client configuration a deployment's settings
// make; everything else keeps the client's defaults.
func (a AuditConfig) ClientConfig() audit.Config {
	return audit.Config{BaseURL: a.BaseURL, APIKey: a.APIKey}
}

// Validate checks that the server configuration is valid.
func (s ServerConfig) Validate() error {
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", s.Port)
	}
	if s.ServiceURL == "" {
		return fmt.Errorf("server.serviceURL is required")
	}
	if err := HTTPURL("server.serviceURL", s.ServiceURL); err != nil {
		return err
	}
	if s.MaxRequestBytes <= 0 {
		return fmt.Errorf("server.maxRequestBytes must be greater than zero")
	}
	if s.ReadHeaderTimeout <= 0 {
		return fmt.Errorf("server.readHeaderTimeout must be greater than zero")
	}
	if s.ReadTimeout <= 0 {
		return fmt.Errorf("server.readTimeout must be greater than zero")
	}
	if s.WriteTimeout <= 0 {
		return fmt.Errorf("server.writeTimeout must be greater than zero")
	}
	if s.IdleTimeout <= 0 {
		return fmt.Errorf("server.idleTimeout must be greater than zero")
	}
	for _, p := range []struct{ key, path string }{
		{"server.servicesConfigPath", s.ServicesConfigPath},
		{"server.paymentMethodsConfigPath", s.PaymentMethodsConfigPath},
		{"server.catalogConfigPath", s.CatalogConfigPath},
	} {
		if strings.TrimSpace(p.path) == "" {
			return fmt.Errorf("%s is required", p.key)
		}
	}
	return nil
}

// Load reads the configuration from the config.yaml at CONFIG_PATH (default
// configs/config.yaml) and validates it. The file is mandatory. Settings the
// server can't run without are checked by Validate, so one left out fails
// here, at startup, rather than when it is first used. Optional settings left
// out take Go's zero value (false, 0, empty), and unrecognised keys are
// ignored.
//
// The server reads only two env vars itself: CONFIG_PATH and APP_ENV. Every
// other setting is in the config file. A secret may still come from the
// environment (or a mounted file), but only where the config file asks for it
// with a "{{env:NAME}}" / "{{file:/path}}" placeholder, which configyaml
// resolves at load.
func Load() (*Config, error) {
	path := strings.TrimSpace(os.Getenv("CONFIG_PATH"))
	if path == "" {
		path = defaultConfigPath
	}
	cfg, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadFile decodes config.yaml at path, resolving its placeholders. It does
// not validate.
func loadFile(path string) (*Config, error) {
	var cfg Config
	if err := configyaml.LoadAndExpand(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks that all required configuration is present.
func (c *Config) Validate() error {
	if err := c.Mode.Validate(); err != nil {
		return err
	}
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("invalid server configuration: %w", err)
	}
	if err := nswdatabase.Validate(c.Database); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("invalid storage configuration: %w", err)
	}
	if err := c.Authn.Validate(); err != nil {
		return fmt.Errorf("invalid authn configuration: %w", err)
	}
	// TNSW's trade integrations; an agency builds none of them.
	if c.Mode != ModeAgency {
		if err := c.Integrations.Validate(); err != nil {
			return fmt.Errorf("invalid external integration configuration: %w", err)
		}
	}
	// Refuse to skip JWKS TLS verification outside development: a forged
	// signing-key response here means full JWT forgery / auth bypass.
	if c.Authn.InsecureSkipTLSVerify && !isDevEnvironment() {
		return fmt.Errorf("authn.insecureSkipTLSVerify: insecure TLS verification requested but APP_ENV is not \"development\" (unset or any other value is treated as production); refusing to start — provide a trusted certificate chain, or set APP_ENV=development for a non-production run")
	}
	// Outbound M2M (services.json) may also disable TLS verification per service;
	// hold it to the same rule so an insecure token endpoint can't ship to prod.
	if err := guardServicesConfigTLS(c.Server.ServicesConfigPath); err != nil {
		return err
	}
	if err := c.Temporal.Validate(); err != nil {
		return fmt.Errorf("invalid temporal configuration: %w", err)
	}
	if err := validateCORS(c.CORS); err != nil {
		return fmt.Errorf("invalid CORS configuration: %w", err)
	}
	if err := c.Notification.Validate(); err != nil {
		return fmt.Errorf("invalid notification configuration: %w", err)
	}
	if err := c.ArtifactLoader.Validate(); err != nil {
		return fmt.Errorf("invalid artifact loader configuration: %w", err)
	}
	return nil
}

// validateCORS runs core/cors's checks, then requires the allowed methods and
// headers too: core/cors accepts them empty, which answers every preflight with
// none allowed, so browsers refuse all but simple requests.
func validateCORS(c cors.Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if len(c.AllowedMethods) == 0 {
		return fmt.Errorf("cors.allowedMethods is required")
	}
	if len(c.AllowedHeaders) == 0 {
		return fmt.Errorf("cors.allowedHeaders is required")
	}
	return nil
}

// servicesTLSProbe is a minimal view of the outbound services registry
// (configs/services*.json) used only to detect per-service TLS-skip flags.
type servicesTLSProbe struct {
	Services []struct {
		ID   string `json:"id"`
		Auth struct {
			Options struct {
				InsecureSkipTLSVerify bool `json:"insecure_skip_tls_verify"`
			} `json:"options"`
		} `json:"auth"`
	} `json:"services"`
}

// guardServicesConfigTLS enforces the APP_ENV=development gate on any per-service
// insecure_skip_tls_verify in the outbound services registry. A missing/empty
// path is not an error here — remote.LoadServices reports that at bootstrap.
func guardServicesConfigTLS(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // G703: path is server.servicesConfigPath from the operator's own config.yaml, not user input.
	if err != nil {
		if os.IsNotExist(err) {
			return nil // absent config: surfaced later by LoadServices
		}
		return fmt.Errorf("read services config %s: %w", path, err)
	}
	var probe servicesTLSProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("parse services config %s: %w", path, err)
	}
	for _, s := range probe.Services {
		if s.Auth.Options.InsecureSkipTLSVerify && !isDevEnvironment() {
			return fmt.Errorf("insecure_skip_tls_verify (services.json service %q): APP_ENV is not \"development\" (unset or any other value is treated as production); refusing to start", s.ID)
		}
	}
	return nil
}

// isDevEnvironment reports whether APP_ENV explicitly designates a development
// run (case-insensitive "development"). Unset or any other value is treated as
// production. This is the only place APP_ENV is read; it exists solely to gate
// the insecure-TLS escape hatches above, which must never be honored outside an
// explicit development run.
func isDevEnvironment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "development")
}
