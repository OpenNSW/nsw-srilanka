package database

import (
	"strings"
	"testing"

	"github.com/OpenNSW/core/database"
)

func validPostgres() database.Config {
	return database.Config{
		Driver: database.Postgres,
		Postgres: &database.PostgresConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "secret",
			Name:     "nsw_db",
			SSLMode:  "require",
		},
	}
}

func TestValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*database.Config)
		wantErr string
	}{
		"complete":          {func(*database.Config) {}, ""},
		"no driver":         {func(c *database.Config) { c.Driver = "" }, "driver is required"},
		"no password":       {func(c *database.Config) { c.Postgres.Password = "" }, "password is required"},
		"no port":           {func(c *database.Config) { c.Postgres.Port = 0 }, "db.postgres.port"},
		"port out of range": {func(c *database.Config) { c.Postgres.Port = 70000 }, "db.postgres.port"},
		"no sslMode":        {func(c *database.Config) { c.Postgres.SSLMode = "" }, "db.postgres.sslMode is required"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validPostgres()
			tc.mutate(&cfg)
			err := Validate(cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}
