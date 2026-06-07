DROP INDEX IF EXISTS idx_task_records_v2_root_workflow_id;
ALTER TABLE task_records_v2 DROP COLUMN root_workflow_id;
