-- Created at: 2026-06-15T07:44:24Z

-- @UP
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS active_output_namespace;

-- @DOWN
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS active_output_namespace TEXT NOT NULL DEFAULT '';
