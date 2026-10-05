package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
	"github.com/OpenNSW/core/temporal"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	nswdatabase "github.com/OpenNSW/nsw-srilanka/internal/database"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// defaultConfigPath is where Load looks for config.yaml when CONFIG_PATH is
// unset.
const defaultConfigPath = "configs/config.yaml"

// defaults returns the configuration a config.yaml that sets nothing yields.
// The file is decoded over it, so each key the file sets replaces one default
// and every key it leaves out keeps its own.
//
// Secrets have no default: they are set in the file, as a placeholder. Neither
// has server.serviceURL or storage.local.publicURL, which default to values
// derived from other settings (see derive).
func defaults() Config {
	return Config{
		Mode:     ModeTNSW,
		Database: nswdatabase.Defaults(),
		Server: ServerConfig{
			Port:                     8080,
			ServicesConfigPath:       "configs/services.json",
			PaymentMethodsConfigPath: "configs/payment_methods.json",
			CatalogConfigPath:        "configs/catalog.json",
			LogLevel:                 slog.LevelInfo,
			MaxRequestBytes:          33554432, // 32 MiB
			ReadHeaderTimeout:        5 * time.Second,
			ReadTimeout:              15 * time.Second,
			WriteTimeout:             30 * time.Second,
			IdleTimeout:              60 * time.Second,
		},
		CORS: cors.Config{
			AllowedOrigins:   []string{},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", "Authorization"},
			AllowCredentials: true,
			MaxAge:           3600,
		},
		Storage: nswstorage.Config{
			Config: storage.Config{
				Type: storage.TypeLocal,
				Local: drivers.LocalConfig{
					BaseDir:   "./bucket",
					PutSecret: "local-dev-secret",
				},
				S3: drivers.S3Config{
					Bucket: "nsw-uploads",
					Region: "us-east-1",
				},
				PresignTTLSeconds: 15 * 60,
			},
			Proxy: nswstorage.ProxyConfig{
				UploadPath:   nswstorage.DefaultProxyUploadPath,
				DownloadPath: nswstorage.DefaultProxyDownloadPath,
				DeletePath:   nswstorage.DefaultProxyDeletePath,
			},
		},
		Authn: authn.Config{
			JWKSURL:   "https://localhost:8090/oauth2/jwks",
			Issuer:    "https://localhost:8090",
			Audience:  "https://api.nsw-srilanka.local",
			ClientIDs: []string{"TRADER_PORTAL_APP", "FCAU_TO_NSW", "NPQS_TO_NSW", "CDA_TO_NSW", "SLPA_TO_NSW", "SLCE_TO_NSW", "GOVPAY_TO_NSW"},
		},
		Temporal: temporal.Config{
			Host:      "localhost",
			Port:      7233,
			Namespace: "default",
		},
		ArtifactLoader: loaders.Config{
			Type:  loaders.TypeLocal,
			Local: local.Config{Root: "configs"},
		},
	}
}

// loadFile decodes config.yaml at path over defaults, resolving its
// placeholders, then fills in the derived settings. It does not validate,
// beyond mode having to name a known Mode.
func loadFile(path string) (*Config, error) {
	cfg := defaults()
	if err := configyaml.LoadAndExpand(path, &cfg); err != nil {
		return nil, err
	}
	mode, err := parseMode(string(cfg.Mode))
	if err != nil {
		return nil, err
	}
	cfg.Mode = mode
	derive(&cfg)
	return &cfg, nil
}

// derive fills the defaults that depend on other settings, for whichever of
// them the file left unset: server.serviceURL from server.port, and
// storage.local.publicURL from server.serviceURL.
func derive(cfg *Config) {
	if strings.TrimSpace(cfg.Server.ServiceURL) == "" {
		cfg.Server.ServiceURL = fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
	}
	if strings.TrimSpace(cfg.Storage.Local.PublicURL) == "" {
		cfg.Storage.Local.PublicURL = cfg.Server.ServiceURL
	}
}
