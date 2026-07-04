// Package ephyto is the National Single Window's in-repo IPPC ePhyto Hub
// integration. The SOAP/mTLS work is invoked directly, in-process, by the
// NPQS ePhyto taskflow plugins (validate / submit / poll — see
// internal/tasks/plugins). No separate HTTP endpoint or auth is involved: the
// trader drives submission and status polling through the standard task
// endpoint, and the plugins perform the Hub calls on the backend's behalf.
//
// This file owns runtime configuration and the mTLS client factory; the
// ePhyto/SOAP building blocks live in the sibling spscert and hub packages
// (ported from the OpenNSW external-integrations-sandbox `ippc` module); the
// friendly-JSON mapping and helpers live in build.go.
package ephyto

import (
	"fmt"
	"os"
	"time"

	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto/hub"
)

// Config is the resolved runtime configuration used to reach the IPPC Hub.
// Every value is sourced from the environment with a single built-in default,
// mirroring the sandbox exporter's precedence (process env > default) so
// operational values never live in source.
type Config struct {
	Endpoint string
	Timeout  time.Duration
	CertPath string
	KeyPath  string
}

const (
	defaultEndpoint = "https://hub.ephytoexchange.org/hub/DeliveryService"
	defaultTimeout  = "60s"
	// mTLS client material, brought over from the external-integrations-sandbox
	// ippc/certs. Overridable via HUB_CERT_PATH / HUB_KEY_PATH; the compose api
	// service bind-mounts these same paths from ./certs/ippc.
	defaultCertPath = "certs/ippc/nppo.crt"
	defaultKeyPath  = "certs/ippc/nppo.key"
)

// LoadConfig reads the Hub configuration from the environment.
func LoadConfig() (*Config, error) {
	timeoutStr := getEnv("HUB_TIMEOUT", defaultTimeout)
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("HUB_TIMEOUT %q is not a valid duration: %w", timeoutStr, err)
	}
	return &Config{
		Endpoint: getEnv("HUB_ENDPOINT", defaultEndpoint),
		Timeout:  timeout,
		CertPath: getEnv("HUB_CERT_PATH", defaultCertPath),
		KeyPath:  getEnv("HUB_KEY_PATH", defaultKeyPath),
	}, nil
}

// NewHubClient builds an mTLS Hub client from the config. The client cert + key
// are required (the Hub authenticates the NPPO by client certificate).
func (c *Config) NewHubClient() (*hub.Client, error) {
	if c.CertPath == "" || c.KeyPath == "" {
		return nil, fmt.Errorf("IPPC Hub client certificate and key are required (set HUB_CERT_PATH / HUB_KEY_PATH)")
	}
	return hub.NewClient(c.Endpoint, c.CertPath, c.KeyPath, c.Timeout)
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
