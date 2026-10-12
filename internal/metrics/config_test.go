package metrics

import (
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	cases := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"off": {},
		"valid URL": {
			cfg: Config{
				Endpoint:    "http://otel-collector:4318/v1/metrics",
				ServiceName: "tnsw-api",
			},
		},
		"no scheme": {
			cfg:     Config{Endpoint: "otel-collector:4318/v1/metrics", ServiceName: "tnsw-api"},
			wantErr: "metrics.endpoint must be a valid absolute http(s) URL",
		},
		"wrong scheme": {
			cfg:     Config{Endpoint: "ftp://otel-collector:4318/v1/metrics", ServiceName: "tnsw-api"},
			wantErr: "metrics.endpoint must be a valid absolute http(s) URL",
		},
		"bare host": {
			cfg:     Config{Endpoint: "http://otel-collector:4318", ServiceName: "tnsw-api"},
			wantErr: "metrics.endpoint must include the OTLP path",
		},
		"root path": {
			cfg:     Config{Endpoint: "http://otel-collector:4318/", ServiceName: "tnsw-api"},
			wantErr: "metrics.endpoint must include the OTLP path",
		},
		"missing service name": {
			cfg:     Config{Endpoint: "http://otel-collector:4318/v1/metrics"},
			wantErr: "metrics.serviceName is required",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}
