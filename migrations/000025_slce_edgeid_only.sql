-- Created at: 2026-10-20T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- SLC Edge's correlation id is edgeId in the interface specification, and the
-- callbacks now match it under that name only.
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

-- These indexes served only the edge_id lookups.
DROP INDEX IF EXISTS idx_task_records_v2_edge_id_snake;
DROP INDEX IF EXISTS idx_task_records_v2_cdn_edge_id_snake;

-- @DOWN
-- The copied edgeId values in task data are left in place.
CREATE INDEX IF NOT EXISTS idx_task_records_v2_edge_id_snake ON task_records_v2 ((data->'cig'->>'edge_id'));
CREATE INDEX IF NOT EXISTS idx_task_records_v2_cdn_edge_id_snake ON task_records_v2 ((data->'cig_cdn'->>'edge_id'));
