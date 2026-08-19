-- Created at: 2026-08-19T09:00:00Z

-- @UP
-- The CDN integration-result and acknowledgment callbacks locate the workflow
-- parked on a dispatch note by the edgeId its dispatch step recorded, under
-- either spelling. These are the CDN counterparts of the indexes 000010 added
-- for the declaration side.
CREATE INDEX IF NOT EXISTS idx_task_records_v2_cdn_edge_id ON task_records_v2 ((data->'cig_cdn'->>'edgeId'));
CREATE INDEX IF NOT EXISTS idx_task_records_v2_cdn_edge_id_snake ON task_records_v2 ((data->'cig_cdn'->>'edge_id'));

-- @DOWN
DROP INDEX IF EXISTS idx_task_records_v2_cdn_edge_id;
DROP INDEX IF EXISTS idx_task_records_v2_cdn_edge_id_snake;
