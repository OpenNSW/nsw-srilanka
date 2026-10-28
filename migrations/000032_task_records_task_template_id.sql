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
-- Postgres adds a column only at the end, so the table is rebuilt to put render_config
-- back in its place. The rows keep everything but their task template.
CREATE TABLE task_records_v2_rollback (
    task_id text NOT NULL,
    task_type text,
    state text,
    render_config jsonb,
    parent_workflow_id text,
    task_workflow_id text,
    active_task_template_id text,
    case_id text DEFAULT ''::text NOT NULL,
    data jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    parent_step_id text,
    active_step_id uuid,
    seq bigint DEFAULT 0 NOT NULL,
    claimed_by text,
    claimed_at timestamp with time zone,
    manifest_version integer DEFAULT 0 NOT NULL
);
INSERT INTO task_records_v2_rollback (
    task_id, task_type, state, parent_workflow_id, task_workflow_id, active_task_template_id,
    case_id, data, created_at, updated_at, parent_step_id, active_step_id, seq, claimed_by,
    claimed_at, manifest_version)
SELECT
    task_id, task_type, state, parent_workflow_id, task_workflow_id, active_task_template_id,
    case_id, data, created_at, updated_at, parent_step_id, active_step_id, seq, claimed_by,
    claimed_at, manifest_version
FROM task_records_v2;
DROP TABLE task_records_v2;
ALTER TABLE task_records_v2_rollback RENAME TO task_records_v2;
ALTER TABLE task_records_v2 ADD CONSTRAINT task_records_v2_pkey PRIMARY KEY (task_id);
CREATE INDEX idx_task_records_v2_case_id ON task_records_v2 (case_id);
CREATE INDEX idx_task_records_v2_parent_workflow_id ON task_records_v2 (parent_workflow_id);
CREATE INDEX idx_task_records_v2_task_workflow_id ON task_records_v2 (task_workflow_id);
CREATE INDEX idx_task_records_v2_slpa_order_slug ON task_records_v2 ((data -> 'so' ->> 'slug'));
CREATE INDEX idx_task_records_v2_slpa_service_order_no ON task_records_v2 ((data -> 'so' ->> 'service_order_no'));
