// Package telemetry exports metrics, logs and traces over OTLP/HTTP when
// otel.endpoint is set in config.yaml.
package telemetry

import (
	"fmt"
	"net/url"
	"strings"
)

// Config is the optional otel section of config.yaml. Empty endpoint leaves
// export off.
type Config struct {
	// Endpoint is the OTLP/HTTP base URL without a signal path
	// (e.g. http://openobserve.nsw-infra-staging.svc.cluster.local:5080/api/default
	// or http://otel-collector:4318). /v1/metrics, /v1/traces and /v1/logs are
	// appended. A legacy full .../v1/metrics URL is accepted and stripped to
	// the base.
	Endpoint string `yaml:"endpoint"`
	// ServiceName is the service.name resource attribute. Required when
	// Endpoint is set so TNSW, CDA and EFC can be told apart.
	ServiceName string `yaml:"serviceName"`
	// Headers are sent with each OTLP export. Secrets go in as
	// "{{env:NAME}}" placeholders in config.yaml (e.g. OpenObserve Basic auth).
	Headers map[string]string `yaml:"headers"`
}

// Enabled reports whether OTLP export is configured.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Endpoint) != ""
}

// Validate checks the section when export is on; an empty endpoint needs
// nothing else.
func (c Config) Validate() error {
	if !c.Enabled() {
		return nil
	}
	base, err := c.baseURL()
	if err != nil {
		return err
	}
	u, err := url.ParseRequestURI(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("otel.endpoint must be a valid absolute http(s) URL")
	}
	if strings.TrimSpace(c.ServiceName) == "" {
		return fmt.Errorf("otel.serviceName is required when otel.endpoint is set")
	}
	return nil
}

// baseURL returns the OTLP/HTTP root used to build signal URLs.
func (c Config) baseURL() (string, error) {
	raw := strings.TrimSpace(c.Endpoint)
	if raw == "" {
		return "", nil
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("otel.endpoint must be a valid absolute http(s) URL")
	}
	path := strings.TrimSuffix(u.Path, "/")
	for _, suffix := range []string{"/v1/metrics", "/v1/traces", "/v1/logs"} {
		if strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix)
			break
		}
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimSuffix(u.String(), "/"), nil
}

func (c Config) signalURL(signal string) (string, error) {
	base, err := c.baseURL()
	if err != nil {
		return "", err
	}
	return base + "/v1/" + signal, nil
}
