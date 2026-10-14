-- Created at: 2026-10-11T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- core/taskflow keys task records by the case their workflow belongs to
-- (store.TaskRecord.CaseID, column case_id): in TNSW the consignment ID, shared by every
-- flow of the consignment. root_workflow_id holds that value and is renamed to match.
ALTER TABLE task_records_v2 RENAME COLUMN root_workflow_id TO case_id;
ALTER INDEX IF EXISTS idx_task_records_v2_root_workflow_id RENAME TO idx_task_records_v2_case_id;

-- @DOWN
ALTER INDEX IF EXISTS idx_task_records_v2_case_id RENAME TO idx_task_records_v2_root_workflow_id;
ALTER TABLE task_records_v2 RENAME COLUMN case_id TO root_workflow_id;
