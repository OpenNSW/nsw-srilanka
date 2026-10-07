-- Created at: 2026-10-05T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- Columns for core taskflow's guarded step writes. parent_step_id names the parent's step (the
-- Activity to complete when the task ends), active_step_id the step the task is on, and seq how far
-- it has advanced; every step write is guarded by active_step_id and seq. The run and node columns
-- dropped below are not used by core taskflow.
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS parent_step_id TEXT;
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS active_step_id UUID NULL;
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS seq BIGINT NOT NULL DEFAULT 0;

ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS parent_run_id;
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS parent_node_id;
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS task_run_id;
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS subtask_node_id;

-- @DOWN
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS subtask_node_id TEXT;
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS task_run_id TEXT;
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS parent_node_id TEXT;
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS parent_run_id TEXT;

ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS seq;
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS active_step_id;
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS parent_step_id;
