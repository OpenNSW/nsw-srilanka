-- Created at: 2026-11-03T17:30:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- attributes holds values about a case that the systems its flows run on may see. TNSW
-- sets it when it creates a consignment, and EXTERNAL_REVIEW sends it with every flow it
-- starts on an agency, so it holds only values every agency may see. An agency merges the
-- attributes a new flow starts with into its own row, by top-level key, and fills the
-- columns flows.clients.caseColumns maps from them. A new key needs no migration.
ALTER TABLE cases ADD COLUMN IF NOT EXISTS attributes jsonb NOT NULL DEFAULT '{}'::jsonb
	CONSTRAINT cases_attributes_check CHECK (jsonb_typeof(attributes) = 'object');

-- @DOWN
ALTER TABLE cases DROP COLUMN IF EXISTS attributes;
