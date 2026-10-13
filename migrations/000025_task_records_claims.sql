-- Created at: 2026-10-10T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- Columns for core taskflow's task claims. claimed_by is who is working the task (the
-- internal user ID of the user who claimed it), claimed_at when they claimed it; both are
-- NULL while the task is unclaimed. They are written only by core's guarded ClaimTask and
-- ReleaseTask, never by the step writes, so a claim lasts for the whole task.
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS claimed_by TEXT NULL;
ALTER TABLE task_records_v2 ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ NULL;

-- @DOWN
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS claimed_at;
ALTER TABLE task_records_v2 DROP COLUMN IF EXISTS claimed_by;
