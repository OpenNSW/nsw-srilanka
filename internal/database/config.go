package database

import "github.com/OpenNSW/core/database"

// Defaults returns the db section of config.yaml that a file setting none of
// it yields. The file is decoded over it, so each key the file sets replaces
// one default and every key it leaves out keeps its own.
//
// The section is core/database's own Config, which has the OpenNSW/agency
// migrator's shape (a driver plus one block per driver), so one db section
// serves the server, the otc CLI and the migrate Job alike. Only postgres is
// supported here (see Open). The password has no default: the file sets it,
// as a placeholder.
func Defaults() database.Config {
	return database.Config{
		Driver: database.Postgres,
		Postgres: &database.PostgresConfig{
			Host:    "localhost",
			Port:    5432,
			User:    "postgres",
			Name:    "nsw_db",
			SSLMode: "require",
			Pool: database.PoolConfig{
				MaxIdleConns:           10,
				MaxOpenConns:           100,
				MaxConnLifetimeSeconds: 3600,
			},
		},
	}
}
