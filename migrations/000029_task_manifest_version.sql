-- Created at: 2026-10-23T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- Each task resolves its artifacts through the manifest version its flow started on
-- (core store.TaskRecord.ManifestVersion, column manifest_version), so every step and
-- render of the task uses it. 0 means none: rows from before this migration have no
-- version, and a task without one fails its next step.
ALTER TABLE task_records_v2 ADD COLUMN manifest_version integer NOT NULL DEFAULT 0;

-- @DOWN
ALTER TABLE task_records_v2 DROP COLUMN manifest_version;
