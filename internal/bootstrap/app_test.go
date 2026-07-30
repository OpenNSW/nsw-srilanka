package bootstrap

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

func TestRedactDBPassword(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		password string
		want     string
	}{
		{
			name:     "empty password",
			errStr:   "connection failed with password secret123",
			password: "",
			want:     "connection failed with password secret123",
		},
		{
			name:     "plain password redacted",
			errStr:   "failed to connect user=postgres password=secret123 host=localhost",
			password: "secret123",
			want:     "failed to connect user=postgres password=[REDACTED] host=localhost",
		},
		{
			name:     "url escaped password redacted",
			errStr:   "postgres://user:p%40ss%23word@localhost:5432/db failed",
			password: "p@ss#word",
			want:     "postgres://user:[REDACTED]@localhost:5432/db failed",
		},
		{
			name:     "userinfo escaped password redacted",
			errStr:   "cannot parse `postgres://user:a%20b%40c@localhost:5432/db`",
			password: "a b@c",
			want:     "cannot parse `postgres://user:[REDACTED]@localhost:5432/db`",
		},
		{
			name:     "password appearing inside the placeholder redacted once",
			errStr:   "failed to connect password=RED host=localhost",
			password: "RED",
			want:     "failed to connect password=[REDACTED] host=localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactDBPassword(tt.errStr, tt.password)
			if got != tt.want {
				t.Errorf("redactDBPassword() = %q, want %q", got, tt.want)
			}
		})
	}
}

// hangupListener accepts and immediately closes connections.
func hangupListener(t *testing.T) (host string, port int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed by cleanup
			}
			_ = conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func TestBuild_RedactsDatabasePasswordOnError(t *testing.T) {
	secretPassword := "super_secret_p@ss_123"
	host, port := hangupListener(t)
	cfg := &config.Config{
		Database: database.Config{
			Host:     host,
			Port:     port,
			Username: "postgres",
			Password: secretPassword,
			Name:     "non_existent_db",
			SSLMode:  "require",
		},
	}

	_, err := Build(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected Build to fail for invalid database connection, got nil")
	}

	errStr := err.Error()
	if strings.Contains(errStr, secretPassword) {
		t.Errorf("Build() error leaked raw sensitive password: %s", errStr)
	}

	if !strings.Contains(errStr, "[REDACTED]") && !strings.Contains(errStr, "failed to connect to database") {
		t.Errorf("Build() error did not indicate connection failure or redaction: %s", errStr)
	}
}
