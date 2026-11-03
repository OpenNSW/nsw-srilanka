-- Created at: 2026-11-03T11:08:34Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- core/taskflow names its tables taskflow_tasks and taskflow_task_roles
-- (gormstore.TaskRecordModel, TaskRoleModel). The task table is renamed, with its
-- indexes, so they match its new name.
ALTER TABLE task_records_v2 RENAME TO taskflow_tasks;
ALTER INDEX IF EXISTS task_records_v2_pkey RENAME TO taskflow_tasks_pkey;
ALTER INDEX IF EXISTS idx_task_records_v2_parent_workflow_id RENAME TO idx_taskflow_tasks_parent_workflow_id;
ALTER INDEX IF EXISTS idx_task_records_v2_task_workflow_id RENAME TO idx_taskflow_tasks_task_workflow_id;
ALTER INDEX IF EXISTS idx_task_records_v2_case_id RENAME TO idx_taskflow_tasks_case_id;
ALTER INDEX IF EXISTS idx_task_records_v2_slpa_order_slug RENAME TO idx_taskflow_tasks_slpa_order_slug;
ALTER INDEX IF EXISTS idx_task_records_v2_slpa_service_order_no RENAME TO idx_taskflow_tasks_slpa_service_order_no;

-- StartTask copies the task template's read.roles to the task, and the store writes one
-- row per role in the same transaction as the task row: without this table, a task whose
-- template lists roles fails to start. case_id is copied from the task, so a case's roles
-- match on (case_id, role) without reading taskflow_tasks; the role index serves listings
-- across every case. Tasks started before this migration have no rows.
CREATE TABLE IF NOT EXISTS taskflow_task_roles (
	task_id text NOT NULL,
	role text NOT NULL,
	case_id text NOT NULL DEFAULT '',
	PRIMARY KEY (task_id, role)
);

CREATE INDEX IF NOT EXISTS idx_taskflow_task_roles_case_role ON taskflow_task_roles (case_id, role);
CREATE INDEX IF NOT EXISTS idx_taskflow_task_roles_role ON taskflow_task_roles (role);

-- @DOWN
DROP TABLE IF EXISTS taskflow_task_roles;

ALTER INDEX IF EXISTS idx_taskflow_tasks_slpa_service_order_no RENAME TO idx_task_records_v2_slpa_service_order_no;
ALTER INDEX IF EXISTS idx_taskflow_tasks_slpa_order_slug RENAME TO idx_task_records_v2_slpa_order_slug;
ALTER INDEX IF EXISTS idx_taskflow_tasks_case_id RENAME TO idx_task_records_v2_case_id;
ALTER INDEX IF EXISTS idx_taskflow_tasks_task_workflow_id RENAME TO idx_task_records_v2_task_workflow_id;
ALTER INDEX IF EXISTS idx_taskflow_tasks_parent_workflow_id RENAME TO idx_task_records_v2_parent_workflow_id;
ALTER INDEX IF EXISTS taskflow_tasks_pkey RENAME TO task_records_v2_pkey;
ALTER TABLE taskflow_tasks RENAME TO task_records_v2;
