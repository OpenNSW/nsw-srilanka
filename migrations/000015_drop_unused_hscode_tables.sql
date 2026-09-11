-- @UP
-- ============================================================================
-- Migration: 000015_drop_unused_hscode_tables.sql
-- Purpose: Drop hs_codes and workflow_template_map. Neither table is read
-- anywhere in the application (workflow_template_map has zero references;
-- hs_codes is never queried from the DB), and their seed data was already
-- removed in 000002/000003/000004/000007. workflow_template_map is dropped
-- first since it holds the FK into hs_codes.
-- ============================================================================
DROP TABLE IF EXISTS workflow_template_map;
DROP TABLE IF EXISTS hs_codes;
-- @DOWN
-- ============================================================================
-- Migration: 000015_drop_unused_hscode_tables.sql
-- Purpose: Recreate hs_codes and workflow_template_map as they were in
-- 000001_initial_schema.sql (empty — their seed data is not restored).
-- ============================================================================
CREATE TABLE IF NOT EXISTS hs_codes (
	id text NOT NULL PRIMARY KEY,
	hs_code varchar(50) NOT NULL UNIQUE,
	description text NOT NULL,
	category varchar(100),
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hs_codes_hs_code ON hs_codes (hs_code);

CREATE TABLE IF NOT EXISTS workflow_template_map (
	id text NOT NULL PRIMARY KEY,
	hs_code_id text NOT NULL REFERENCES hs_codes(id) ON UPDATE CASCADE ON DELETE RESTRICT,
	consignment_flow varchar(50) NOT NULL
		CONSTRAINT workflow_template_map_consignment_flow_check
			CHECK ((consignment_flow)::text = ANY ((ARRAY['IMPORT'::character varying, 'EXPORT'::character varying])::text[])),
	workflow_template_id text NOT NULL,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workflow_template_map_hs_code_id ON workflow_template_map (hs_code_id);
