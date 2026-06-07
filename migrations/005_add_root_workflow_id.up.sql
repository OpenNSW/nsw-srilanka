ALTER TABLE task_records_v2 ADD COLUMN root_workflow_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_task_records_v2_root_workflow_id ON task_records_v2(root_workflow_id);
