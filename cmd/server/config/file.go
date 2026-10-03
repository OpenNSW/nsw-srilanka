package config

import (
	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/refid"
)

// fileConfig is the on-disk shape of config.yaml (CONFIG_PATH). Any value may
// be a "{{env:NAME}}" or "{{file:/path}}" placeholder, so secrets never need to
// be written into the file itself.
type fileConfig struct {
	// Mode is the kind of system this deployment runs as: "tnsw" (the default when
	// omitted) or "agency". Parsed and validated by Load.
	Mode string `yaml:"mode"`

	// RefID holds the reference ID formats the REFID_GENERATOR task plugin
	// generates from. Optional: with no issuers, generation is disabled and a
	// template using the plugin fails at run time (see bootstrap.initRefIDs).
	RefID refid.Config `yaml:"refid"`
}

// loadConfigFile reads config.yaml at path, resolving its placeholders. The
// file itself is mandatory, but an empty one is valid and yields the zero
// config.
func loadConfigFile(path string) (fileConfig, error) {
	var fc fileConfig
	if err := configyaml.LoadAndExpand(path, &fc); err != nil {
		return fileConfig{}, err
	}
	return fc, nil
}
