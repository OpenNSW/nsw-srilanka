package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/database"

	nswdatabase "github.com/OpenNSW/nsw-srilanka/internal/database"
)

// defaultConfigPath is where Load looks for the config file when CONFIG_PATH
// is unset — the server's own default, so a checkout runs both on one file.
const defaultConfigPath = "configs/config.yaml"

// Config holds the configuration for the otc CLI: database access only.
//
// It has the server's config.yaml shape, so otc can run on the very file the
// server does — the other sections are simply not decoded — or on a separate
// file that holds just a db section. It intentionally does not share
// cmd/server/config, which also requires and validates unrelated server
// settings (auth, CORS, temporal, artifact loading, etc.) that this CLI never
// uses.
type Config struct {
	Database database.Config `yaml:"db"`
}

// Load reads the config file at CONFIG_PATH (default configs/config.yaml) and
// validates the database settings.
//
// Every "{{env:NAME}}" / "{{file:/path}}" placeholder in the file is resolved,
// not only the db section's, so on the server's config.yaml the secrets
// its other sections reference must be set too.
func Load() (*Config, error) {
	path := strings.TrimSpace(os.Getenv("CONFIG_PATH"))
	if path == "" {
		path = defaultConfigPath
	}
	return loadFile(path)
}

// loadFile decodes the config file at path and validates its db section,
// which has no built-in defaults: every setting otc connects with is set in it.
func loadFile(path string) (*Config, error) {
	cfg := &Config{}
	if err := configyaml.LoadAndExpand(path, cfg); err != nil {
		return nil, err
	}
	if err := nswdatabase.Validate(cfg.Database); err != nil {
		return nil, fmt.Errorf("invalid database configuration: %w", err)
	}

	return cfg, nil
}
