-- Created at: 2026-10-23T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- A case flow runs on the artifact manifest version current when its row was first
-- recorded (workflow.VarManifestVersion). Recording it on the row means a retried start
-- uses the same version, not a newer current one. 0 means none: rows from before this
-- migration have no version.
ALTER TABLE case_flows ADD COLUMN manifest_version integer NOT NULL DEFAULT 0;

-- @DOWN
ALTER TABLE case_flows DROP COLUMN manifest_version;
