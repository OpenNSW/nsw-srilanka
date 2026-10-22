-- Created at: 2026-10-20T01:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- SLC Edge callbacks used to find the waiting workflow by searching task data
-- for the edge_id. A declaration or dispatch note now records the case and the
-- workflow it was dispatched from when SLC Edge accepts it, and the callbacks
-- read the waiting task through the task store by those.
ALTER TABLE cusdec_declarations ADD COLUMN IF NOT EXISTS case_id TEXT;
ALTER TABLE cusdec_declarations ADD COLUMN IF NOT EXISTS workflow_id TEXT;
ALTER TABLE dispatch_notes ADD COLUMN IF NOT EXISTS case_id TEXT;
ALTER TABLE dispatch_notes ADD COLUMN IF NOT EXISTS workflow_id TEXT;

-- Link what was dispatched before this, once, from the task data the lookups
-- used to search. Every task that carries an edge_id belongs to the workflow
-- that dispatched it, so any one of them names it.
UPDATE cusdec_declarations d
SET case_id = t.case_id, workflow_id = t.parent_workflow_id
FROM (
	SELECT DISTINCT ON (data->'cig'->>'edge_id')
		data->'cig'->>'edge_id' AS edge_id, case_id, parent_workflow_id
	FROM task_records_v2
	WHERE data->'cig'->>'edge_id' IS NOT NULL
	ORDER BY data->'cig'->>'edge_id', updated_at DESC
) t
WHERE d.edge_id = t.edge_id AND d.workflow_id IS NULL;

UPDATE dispatch_notes n
SET case_id = t.case_id, workflow_id = t.parent_workflow_id
FROM (
	SELECT DISTINCT ON (data->'cig_cdn'->>'edge_id')
		data->'cig_cdn'->>'edge_id' AS edge_id, case_id, parent_workflow_id
	FROM task_records_v2
	WHERE data->'cig_cdn'->>'edge_id' IS NOT NULL
	ORDER BY data->'cig_cdn'->>'edge_id', updated_at DESC
) t
WHERE n.edge_id = t.edge_id AND n.workflow_id IS NULL;

-- A dispatch still waiting for its first callback has no row yet: it used to be
-- created when the callback arrived. Record it as submitted, as a dispatch now
-- is, so that callback finds its workflow.
INSERT INTO cusdec_declarations (id, edge_id, status, case_id, workflow_id)
SELECT DISTINCT ON (data->'cig'->>'edge_id')
	gen_random_uuid()::text, data->'cig'->>'edge_id', 'SUBMITTED', case_id, parent_workflow_id
FROM task_records_v2 t
WHERE data->'cig'->>'edge_id' IS NOT NULL
	AND NOT EXISTS (SELECT 1 FROM cusdec_declarations d WHERE d.edge_id = t.data->'cig'->>'edge_id')
ORDER BY data->'cig'->>'edge_id', updated_at DESC;

INSERT INTO dispatch_notes (id, edge_id, status, case_id, workflow_id)
SELECT DISTINCT ON (data->'cig_cdn'->>'edge_id')
	gen_random_uuid()::text, data->'cig_cdn'->>'edge_id', 'SUBMITTED', case_id, parent_workflow_id
FROM task_records_v2 t
WHERE data->'cig_cdn'->>'edge_id' IS NOT NULL
	AND NOT EXISTS (SELECT 1 FROM dispatch_notes n WHERE n.edge_id = t.data->'cig_cdn'->>'edge_id')
ORDER BY data->'cig_cdn'->>'edge_id', updated_at DESC;

-- Nothing searches task data for an edge_id any more.
DROP INDEX IF EXISTS idx_task_records_v2_edge_id_snake;
DROP INDEX IF EXISTS idx_task_records_v2_cdn_edge_id_snake;

-- @DOWN
CREATE INDEX IF NOT EXISTS idx_task_records_v2_edge_id_snake ON task_records_v2 ((data->'cig'->>'edge_id'));
CREATE INDEX IF NOT EXISTS idx_task_records_v2_cdn_edge_id_snake ON task_records_v2 ((data->'cig_cdn'->>'edge_id'));
ALTER TABLE dispatch_notes DROP COLUMN IF EXISTS workflow_id;
ALTER TABLE dispatch_notes DROP COLUMN IF EXISTS case_id;
ALTER TABLE cusdec_declarations DROP COLUMN IF EXISTS workflow_id;
ALTER TABLE cusdec_declarations DROP COLUMN IF EXISTS case_id;
