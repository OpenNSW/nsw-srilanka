-- Created at: 2026-10-11T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- A consignment is a case: its name and state live on the cases row with the same id
-- (see internal/caseflow), which keeps the state in step with the consignment's flows.
-- consignments keeps only the trade data. Each existing consignment gets its cases row
-- first, so the foreign key holds; cases has no INITIALIZED state, so anything not
-- FINISHED is IN_PROGRESS.
INSERT INTO cases (id, name, state, created_at, updated_at)
SELECT id, name, CASE WHEN state = 'FINISHED' THEN 'FINISHED' ELSE 'IN_PROGRESS' END, created_at, updated_at
FROM consignments
ON CONFLICT (id) DO NOTHING;

ALTER TABLE consignments
	ADD CONSTRAINT consignments_id_fkey FOREIGN KEY (id) REFERENCES cases (id);

-- Drops idx_consignments_state and consignments_state_check with the column.
ALTER TABLE consignments DROP COLUMN IF EXISTS state;
ALTER TABLE consignments DROP COLUMN IF EXISTS name;

-- @DOWN
ALTER TABLE consignments ADD COLUMN IF NOT EXISTS name varchar(255);
ALTER TABLE consignments ADD COLUMN IF NOT EXISTS state varchar(50);

UPDATE consignments SET name = cases.name, state = cases.state
FROM cases
WHERE cases.id = consignments.id;

ALTER TABLE consignments ALTER COLUMN state SET NOT NULL;
ALTER TABLE consignments ADD CONSTRAINT consignments_state_check
	CHECK ((state)::text = ANY (ARRAY['INITIALIZED'::character varying, 'IN_PROGRESS'::character varying, 'FINISHED'::character varying]::text[]));
CREATE INDEX IF NOT EXISTS idx_consignments_state ON consignments (state);

ALTER TABLE consignments DROP CONSTRAINT IF EXISTS consignments_id_fkey;
