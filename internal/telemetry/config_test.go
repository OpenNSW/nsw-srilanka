package telemetry

import (
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "disabled", cfg: Config{}},
		{
			name: "openobserve org base",
			cfg: Config{
				Endpoint:    "http://openobserve.nsw-infra-staging.svc.cluster.local:5080/api/default",
				ServiceName: "tnsw-api",
			},
		},
		{
			name: "collector base",
			cfg:  Config{Endpoint: "http://otel-collector:4318", ServiceName: "tnsw-api"},
		},
		{
			name: "legacy metrics path",
			cfg:  Config{Endpoint: "http://otel-collector:4318/v1/metrics", ServiceName: "tnsw-api"},
		},
		{
			name:    "not a url",
			cfg:     Config{Endpoint: "otel-collector:4318", ServiceName: "tnsw-api"},
			wantErr: "otel.endpoint must be a valid absolute http(s) URL",
		},
		{
			name:    "missing service name",
			cfg:     Config{Endpoint: "http://otel-collector:4318"},
			wantErr: "otel.serviceName is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSignalURL(t *testing.T) {
	cfg := Config{Endpoint: "http://openobserve:5080/api/default/", ServiceName: "tnsw-api"}
	got, err := cfg.signalURL("traces")
	if err != nil {
		t.Fatalf("signalURL: %v", err)
	}
	if got != "http://openobserve:5080/api/default/v1/traces" {
		t.Fatalf("signalURL = %q", got)
	}

	cfg.Endpoint = "http://otel-collector:4318/v1/metrics"
	got, err = cfg.signalURL("logs")
	if err != nil {
		t.Fatalf("signalURL: %v", err)
	}
	if got != "http://otel-collector:4318/v1/logs" {
		t.Fatalf("signalURL = %q", got)
	}
}
