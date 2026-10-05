package database

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaults_FileOverridesKeys(t *testing.T) {
	cfg := Defaults()
	if err := yaml.Unmarshal([]byte(`
driver: postgres
postgres:
  host: nsw-db
  port: 6543
  user: nsw
  password: secret
  pool:
    maxOpenConns: 50
`), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	pg := cfg.Postgres
	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"Host", pg.Host, "nsw-db"},
		{"Port", pg.Port, 6543},
		{"User", pg.User, "nsw"},
		{"Password", pg.Password, "secret"},
		{"Pool.MaxOpenConns", pg.Pool.MaxOpenConns, 50},
		// Left out of the file, so the defaults.
		{"Name", pg.Name, "nsw_db"},
		{"SSLMode", pg.SSLMode, "require"},
		{"Pool.MaxIdleConns", pg.Pool.MaxIdleConns, 10},
		{"Pool.MaxConnLifetimeSeconds", pg.Pool.MaxConnLifetimeSeconds, 3600},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// Defaults hands out a fresh postgres block each call, so decoding a file over
// one result cannot leak into the next.
func TestDefaults_Independent(t *testing.T) {
	a, b := Defaults(), Defaults()
	a.Postgres.Host = "changed"
	if b.Postgres.Host != "localhost" {
		t.Fatalf("Defaults() results share a postgres block")
	}
}
