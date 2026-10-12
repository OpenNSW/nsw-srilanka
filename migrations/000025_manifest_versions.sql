-- Created at: 2026-10-12T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- Each flow resolves its artifacts through the manifest version current when it started
-- (core artifact.Set, workflow.VarManifestVersion), so a deploy never changes the templates
-- of a running flow. The version is recorded on the row that starts a flow, so a retried
-- start uses the same one, and on each task (core store.TaskRecord.ManifestVersion, column
-- manifest_version), so every step and render of the task uses it. 0 means none: rows from
-- before this migration have no version, and a task without one fails its next step.
ALTER TABLE case_flows ADD COLUMN manifest_version integer NOT NULL DEFAULT 0;
ALTER TABLE agency_workflow ADD COLUMN manifest_version integer NOT NULL DEFAULT 0;
ALTER TABLE task_records_v2 ADD COLUMN manifest_version integer NOT NULL DEFAULT 0;

-- @DOWN
ALTER TABLE task_records_v2 DROP COLUMN manifest_version;
ALTER TABLE agency_workflow DROP COLUMN manifest_version;
ALTER TABLE case_flows DROP COLUMN manifest_version;
