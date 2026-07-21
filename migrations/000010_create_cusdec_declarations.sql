-- Created at: 2026-07-21T11:00:00Z

-- @UP
CREATE TABLE IF NOT EXISTS cusdec_declarations (
	id TEXT NOT NULL PRIMARY KEY,
	edge_id TEXT NOT NULL,
	status TEXT NOT NULL,
	cusdec_year TEXT,
	cusdec_office TEXT,
	cusdec_serial TEXT,
	cusdec_number INTEGER,
	errors JSONB,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cusdec_declarations_edge_id ON cusdec_declarations (edge_id);
CREATE INDEX IF NOT EXISTS idx_cusdec_declarations_status ON cusdec_declarations (status);
CREATE INDEX IF NOT EXISTS idx_cusdec_ref ON cusdec_declarations (cusdec_year, cusdec_office, cusdec_serial, cusdec_number);

-- Indexes to optimize JSONB lookups on task_records_v2 edgeId
CREATE INDEX IF NOT EXISTS idx_task_records_v2_edge_id ON task_records_v2 ((data->'cig'->>'edgeId'));
CREATE INDEX IF NOT EXISTS idx_task_records_v2_edge_id_snake ON task_records_v2 ((data->'cig'->>'edge_id'));

-- @DOWN
DROP INDEX IF EXISTS idx_task_records_v2_edge_id;
DROP INDEX IF EXISTS idx_task_records_v2_edge_id_snake;
DROP TABLE IF EXISTS cusdec_declarations;
