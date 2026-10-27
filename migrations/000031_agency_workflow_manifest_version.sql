-- Created at: 2026-10-23T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- An agency workflow runs on the artifact manifest version current when its inject was
-- first recorded (workflow.VarManifestVersion). Recording it on the row means a retried
-- inject uses the same version, and the case view shows each task's config from it.
-- 0 means none: rows from before this migration have no version.
ALTER TABLE agency_workflow ADD COLUMN manifest_version integer NOT NULL DEFAULT 0;

-- @DOWN
ALTER TABLE agency_workflow DROP COLUMN manifest_version;
