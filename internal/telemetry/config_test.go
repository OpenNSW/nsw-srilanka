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

func TestEndpointRejectsDiscardedComponentsAndMalformedSignalPaths(t *testing.T) {
	for _, endpoint := range []string{
		"https://collector.example?token=secret",
		"https://collector.example?",
		"https://collector.example/v1/logs?token=secret",
		"https://collector.example#fragment",
		"https://collector.example#",
		"https://collector.example/v1/metrics#fragment",
		"https://collector.example/v1",
		"https://collector.example/v1/metric",
		"https://collector.example/api/default/v1/unknown",
		"https://collector.example/api/default/v1/traces/extra",
		"https://collector.example/v1/metrics/v1/logs",
		"https://collector.example/api/%76%31/metrics",
	} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := Config{Endpoint: endpoint, ServiceName: "tnsw-api"}
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate accepted an endpoint that loses data or has a malformed signal path")
			} else if strings.Contains(err.Error(), "secret") {
				t.Fatal("validation error exposed an endpoint credential")
			}
			if _, err := cfg.signalURL("logs"); err == nil {
				t.Fatal("signalURL accepted an invalid endpoint without validation")
			}
		})
	}
}

func TestSignalURLsPreserveSupportedBases(t *testing.T) {
	for _, base := range []string{
		"http://otel-collector:4318",
		"https://openobserve.example/api/default",
		"https://proxy.example/tenant/telemetry",
		"https://proxy.example/api/org%2Fteam",
		"https://proxy.example/api/org%2Fv1%2Fteam",
	} {
		for _, suffix := range []string{"", "/", "/v1/metrics", "/v1/traces", "/v1/logs", "/v1/logs/"} {
			t.Run(base+suffix, func(t *testing.T) {
				cfg := Config{Endpoint: base + suffix, ServiceName: "tnsw-api"}
				if err := cfg.Validate(); err != nil {
					t.Fatalf("Validate: %v", err)
				}
				for _, signal := range []string{"metrics", "traces", "logs"} {
					got, err := cfg.signalURL(signal)
					if err != nil {
						t.Fatal(err)
					}
					if want := base + "/v1/" + signal; got != want {
						t.Fatalf("signalURL = %q, want %q", got, want)
					}
				}
			})
		}
	}
}
