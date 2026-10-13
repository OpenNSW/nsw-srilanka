package metrics

import (
	"fmt"
	"net/url"
	"strings"
)

// Config is the optional metrics section of config.yaml. Empty endpoint
// leaves metrics off.
type Config struct {
	// Endpoint is the full OTLP/HTTP metrics URL, path included
	// (e.g. http://otel-collector:4318/v1/metrics). Required when metrics
	// are on: the exporter posts to the URL as given.
	Endpoint string `yaml:"endpoint"`
	// ServiceName is the service.name resource attribute. Required when
	// Endpoint is set so TNSW, CDA and EFC can be told apart.
	ServiceName string `yaml:"serviceName"`
	// Headers are sent with each OTLP export. Secrets go in as
	// "{{env:NAME}}" placeholders in config.yaml.
	Headers map[string]string `yaml:"headers"`
}

// Enabled reports whether metrics export is configured.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Endpoint) != ""
}

// Validate checks the section when metrics are on; an empty endpoint needs
// nothing else.
func (c Config) Validate() error {
	if !c.Enabled() {
		return nil
	}
	u, err := url.ParseRequestURI(strings.TrimSpace(c.Endpoint))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("metrics.endpoint must be a valid absolute http(s) URL")
	}
	// OTLP/HTTP posts to the URL as given, so a bare host:port would hit "/".
	if u.Path == "" || u.Path == "/" {
		return fmt.Errorf("metrics.endpoint must include the OTLP path, e.g. http://otel-collector:4318/v1/metrics")
	}
	if strings.TrimSpace(c.ServiceName) == "" {
		return fmt.Errorf("metrics.serviceName is required when metrics.endpoint is set")
	}
	return nil
}
