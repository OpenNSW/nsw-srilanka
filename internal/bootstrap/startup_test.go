package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/internal/telemetry"
	"go.opentelemetry.io/otel"
	collectorlog "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/proto"
)

func TestBuildExportsStartupLogsBeforeReturningDatabaseFailure(t *testing.T) {
	restoreStartupGlobals(t)
	var mu sync.Mutex
	var records []*logspb.LogRecord
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/logs" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var request collectorlog.ExportLogsServiceRequest
			if err := proto.Unmarshal(body, &request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			for _, resource := range request.GetResourceLogs() {
				for _, scope := range resource.GetScopeLogs() {
					records = append(records, scope.GetLogRecords()...)
				}
			}
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collector.Close()

	cfg := &config.Config{Database: database.Config{Postgres: &database.PostgresConfig{}}, Otel: telemetry.Config{Endpoint: collector.URL, ServiceName: "startup-test"}}
	app, err := Build(context.Background(), cfg) // No database driver: fail without external services.
	if app != nil || err == nil || !strings.Contains(err.Error(), "unsupported database driver") {
		t.Fatalf("Build = (%v, %v), want a database failure", app, err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := map[string]bool{
		"configuration loaded successfully": false,
		"CORS configuration":                false,
		"server configuration":              false,
		"failed to bootstrap application":   false,
	}
	for _, record := range records {
		message := record.GetBody().GetStringValue()
		if _, ok := want[message]; !ok {
			continue
		}
		if want[message] {
			t.Errorf("duplicate startup record %q", message)
		}
		want[message] = true
		assertStartupRecord(t, record, err)
	}
	for message, found := range want {
		if !found {
			t.Errorf("startup log %q was not exported before Build returned", message)
		}
	}
}

func assertStartupRecord(t *testing.T, record *logspb.LogRecord, err error) {
	t.Helper()
	message := record.GetBody().GetStringValue()
	attrs := map[string]string{}
	for _, attr := range record.GetAttributes() {
		attrs[attr.GetKey()] = attr.GetValue().GetStringValue()
	}
	if attrs["category"] != "system" {
		t.Errorf("%q category = %q, want system", message, attrs["category"])
	}
	if message == "failed to bootstrap application" {
		if record.GetSeverityNumber() != logspb.SeverityNumber_SEVERITY_NUMBER_ERROR || attrs["exception.message"] != err.Error() {
			t.Errorf("failure record lost severity or error: %v", record)
		}
	}
}

func TestBuildFailureWithoutTelemetry(t *testing.T) {
	logger := slog.Default()
	correlationID := httputil.CorrelationIDFunc
	t.Cleanup(func() {
		slog.SetDefault(logger)
		httputil.CorrelationIDFunc = correlationID
	})
	for _, endpoint := range []string{"", ":invalid"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := &config.Config{Database: database.Config{Postgres: &database.PostgresConfig{}}, Otel: telemetry.Config{Endpoint: endpoint, ServiceName: "startup-test"}}
			app, err := Build(context.Background(), cfg)
			if app != nil || err == nil {
				t.Fatalf("Build = (%v, %v), want a startup failure", app, err)
			}
			want := "failed to connect to database"
			if endpoint != "" {
				want = "failed to start telemetry"
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("Build error = %v, want %q", err, want)
			}
		})
	}
}

func TestBuildFailureWithUnavailableCollector(t *testing.T) {
	restoreStartupGlobals(t)
	attempted := make(chan struct{}, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/logs" {
			select {
			case attempted <- struct{}{}:
			default:
			}
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer collector.Close()
	cfg := &config.Config{
		Database: database.Config{Postgres: &database.PostgresConfig{}},
		Otel:     telemetry.Config{Endpoint: collector.URL, ServiceName: "startup-test"},
	}
	start := time.Now()
	app, err := Build(context.Background(), cfg)
	if app != nil || err == nil || !strings.Contains(err.Error(), "unsupported database driver") {
		t.Fatalf("Build = (%v, %v), want the original database failure", app, err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Build took %v with an unavailable collector; shutdown should be bounded", elapsed)
	}
	select {
	case <-attempted:
	default:
		t.Error("startup logs were not sent to the collector")
	}
}

func restoreStartupGlobals(t *testing.T) {
	t.Helper()
	logger := slog.Default()
	correlationID := httputil.CorrelationIDFunc
	tracer := otel.GetTracerProvider()
	propagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(logger)
		httputil.CorrelationIDFunc = correlationID
		if otel.GetTracerProvider() != tracer {
			otel.SetTracerProvider(tracer)
		}
		if otel.GetTextMapPropagator() != propagator {
			otel.SetTextMapPropagator(propagator)
		}
	})
}
