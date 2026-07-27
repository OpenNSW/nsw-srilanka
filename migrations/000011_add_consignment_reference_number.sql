-- Created at: 2026-07-27T00:00:00Z

-- @UP
ALTER TABLE consignments ADD COLUMN IF NOT EXISTS reference_number VARCHAR(100);
CREATE UNIQUE INDEX IF NOT EXISTS idx_consignments_reference_number ON consignments (reference_number);

CREATE SEQUENCE IF NOT EXISTS consignment_ref_seq START WITH 1;

CREATE TABLE IF NOT EXISTS consignment_ref_counters (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- @DOWN
DROP INDEX IF EXISTS idx_consignments_reference_number;
ALTER TABLE consignments DROP COLUMN IF EXISTS reference_number;
DROP SEQUENCE IF EXISTS consignment_ref_seq;
DROP TABLE IF EXISTS consignment_ref_counters;
