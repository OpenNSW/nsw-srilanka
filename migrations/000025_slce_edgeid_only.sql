-- Created at: 2026-10-20T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- Callbacks locate the waiting task by the id its dispatch step returned, which
-- the customs workflows record as cig.edge_id / cig_cdn.edge_id. The lookups
-- used to accept edgeId as well and now match edge_id only. Give every task that
-- holds only edgeId the edge_id it stands for, so a callback for a declaration
-- or dispatch note already waiting still finds it.
UPDATE task_records_v2
SET data = jsonb_set(data, '{cig,edge_id}', data->'cig'->'edgeId')
WHERE jsonb_typeof(data->'cig') = 'object'
  AND data->'cig' ? 'edgeId'
  AND NOT data->'cig' ? 'edge_id';

UPDATE task_records_v2
SET data = jsonb_set(data, '{cig_cdn,edge_id}', data->'cig_cdn'->'edgeId')
WHERE jsonb_typeof(data->'cig_cdn') = 'object'
  AND data->'cig_cdn' ? 'edgeId'
  AND NOT data->'cig_cdn' ? 'edge_id';

-- These indexes served only the edgeId lookups.
DROP INDEX IF EXISTS idx_task_records_v2_edge_id;
DROP INDEX IF EXISTS idx_task_records_v2_cdn_edge_id;

-- @DOWN
-- The copied edge_id values in task data are left in place.
CREATE INDEX IF NOT EXISTS idx_task_records_v2_edge_id ON task_records_v2 ((data->'cig'->>'edgeId'));
CREATE INDEX IF NOT EXISTS idx_task_records_v2_cdn_edge_id ON task_records_v2 ((data->'cig_cdn'->>'edgeId'));
