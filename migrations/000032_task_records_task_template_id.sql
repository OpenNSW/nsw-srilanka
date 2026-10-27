-- Created at: 2026-10-27T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- A task record names its task template (store.TaskRecord.TaskTemplateID) instead of
-- holding a copy of its render config: every render resolves the config through the
-- template, in the manifest version the task runs on (core's renderconfig.Load).
-- Rows from before this migration have no template, so they no longer render; nothing
-- is in production, and they are cleared at the cutover.
ALTER TABLE task_records_v2 ADD COLUMN task_template_id text NOT NULL DEFAULT '';
ALTER TABLE task_records_v2 DROP COLUMN render_config;

-- @DOWN
ALTER TABLE task_records_v2 ADD COLUMN render_config jsonb;
ALTER TABLE task_records_v2 DROP COLUMN task_template_id;
