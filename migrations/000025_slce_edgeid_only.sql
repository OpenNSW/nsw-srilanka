-- Created at: 2026-10-20T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- Callbacks locate the waiting task by the id its dispatch step returned, which
-- the customs workflows record as cig.edge_id / cig_cdn.edge_id. The lookups
-- used to accept edgeId as well and now match edge_id only. Rename edgeId to
-- edge_id in every task that holds it, so a callback for a declaration or
-- dispatch note already waiting still finds it. A task that holds both keeps its
-- edge_id.
UPDATE task_records_v2
SET data = jsonb_set(data #- '{cig,edgeId}', '{cig,edge_id}',
	COALESCE(data->'cig'->'edge_id', data->'cig'->'edgeId'))
WHERE jsonb_typeof(data->'cig') = 'object'
  AND data->'cig' ? 'edgeId';

UPDATE task_records_v2
SET data = jsonb_set(data #- '{cig_cdn,edgeId}', '{cig_cdn,edge_id}',
	COALESCE(data->'cig_cdn'->'edge_id', data->'cig_cdn'->'edgeId'))
WHERE jsonb_typeof(data->'cig_cdn') = 'object'
  AND data->'cig_cdn' ? 'edgeId';

-- These indexes served only the edgeId lookups.
DROP INDEX IF EXISTS idx_task_records_v2_edge_id;
DROP INDEX IF EXISTS idx_task_records_v2_cdn_edge_id;

-- @DOWN
-- Task data keeps edge_id: the lookups before this matched either key.
CREATE INDEX IF NOT EXISTS idx_task_records_v2_edge_id ON task_records_v2 ((data->'cig'->>'edgeId'));
CREATE INDEX IF NOT EXISTS idx_task_records_v2_cdn_edge_id ON task_records_v2 ((data->'cig_cdn'->>'edgeId'));
