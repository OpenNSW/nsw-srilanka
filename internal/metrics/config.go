package metrics

import "strings"

// Config is the optional metrics section of config.yaml. Empty endpoint
// leaves metrics off.
type Config struct {
	// Endpoint is the full OTLP/HTTP metrics URL, path included.
	Endpoint string `yaml:"endpoint"`
	// ServiceName is the service.name resource attribute. Required when
	// Endpoint is set so TNSW, CDA and Customs can be told apart.
	ServiceName string `yaml:"serviceName"`
	// Headers are sent with each OTLP export. Secrets go in as
	// "{{env:NAME}}" placeholders in config.yaml.
	Headers map[string]string `yaml:"headers"`
}

// Enabled reports whether metrics export is configured.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Endpoint) != ""
}
