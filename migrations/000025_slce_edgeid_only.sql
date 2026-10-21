-- Created at: 2026-10-20T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- SLC Edge's correlation id is edgeId in the interface specification. The
-- callbacks now match it under that name only, and the tables that store it
-- name the column after it.
--
-- Callbacks locate the waiting task by the edgeId its dispatch step recorded.
-- The customs workflows used to copy it to cig.edge_id / cig_cdn.edge_id
-- (one-trade-artifacts#208 keeps it as edgeId), and the lookups no longer
-- accept that spelling. Give every task that holds only edge_id the edgeId it
-- stands for, so a callback for a declaration or dispatch note already waiting
-- still finds it.
UPDATE task_records_v2
SET data = jsonb_set(data, '{cig,edgeId}', data->'cig'->'edge_id')
WHERE jsonb_typeof(data->'cig') = 'object'
  AND data->'cig' ? 'edge_id'
  AND NOT data->'cig' ? 'edgeId';

UPDATE task_records_v2
SET data = jsonb_set(data, '{cig_cdn,edgeId}', data->'cig_cdn'->'edge_id')
WHERE jsonb_typeof(data->'cig_cdn') = 'object'
  AND data->'cig_cdn' ? 'edge_id'
  AND NOT data->'cig_cdn' ? 'edgeId';

DROP INDEX IF EXISTS idx_task_records_v2_edge_id_snake;
DROP INDEX IF EXISTS idx_task_records_v2_cdn_edge_id_snake;
ALTER INDEX IF EXISTS idx_task_records_v2_edge_id RENAME TO idx_task_records_v2_cig_edgeid;
ALTER INDEX IF EXISTS idx_task_records_v2_cdn_edge_id RENAME TO idx_task_records_v2_cig_cdn_edgeid;

-- The declaration and dispatch-note tables keep the column under the same
-- name. Quoted: unquoted, Postgres would fold it to edgeid.
ALTER TABLE cusdec_declarations RENAME COLUMN edge_id TO "edgeId";
ALTER INDEX IF EXISTS idx_cusdec_declarations_edge_id RENAME TO idx_cusdec_declarations_edgeid;
ALTER TABLE dispatch_notes RENAME COLUMN edge_id TO "edgeId";
ALTER INDEX IF EXISTS idx_dispatch_notes_edge_id RENAME TO idx_dispatch_notes_edgeid;

-- @DOWN
-- The copied edgeId values in task data are left in place.
ALTER INDEX IF EXISTS idx_dispatch_notes_edgeid RENAME TO idx_dispatch_notes_edge_id;
ALTER TABLE dispatch_notes RENAME COLUMN "edgeId" TO edge_id;
ALTER INDEX IF EXISTS idx_cusdec_declarations_edgeid RENAME TO idx_cusdec_declarations_edge_id;
ALTER TABLE cusdec_declarations RENAME COLUMN "edgeId" TO edge_id;

ALTER INDEX IF EXISTS idx_task_records_v2_cig_cdn_edgeid RENAME TO idx_task_records_v2_cdn_edge_id;
ALTER INDEX IF EXISTS idx_task_records_v2_cig_edgeid RENAME TO idx_task_records_v2_edge_id;
CREATE INDEX IF NOT EXISTS idx_task_records_v2_edge_id_snake ON task_records_v2 ((data->'cig'->>'edge_id'));
CREATE INDEX IF NOT EXISTS idx_task_records_v2_cdn_edge_id_snake ON task_records_v2 ((data->'cig_cdn'->>'edge_id'));
