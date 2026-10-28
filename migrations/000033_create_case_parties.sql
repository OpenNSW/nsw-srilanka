-- Created at: 2026-10-28T11:49:25Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- case_parties records who holds which logical role on a case: in TNSW the consignment's
-- trader company and the CHA company it selected. role is a catalog logical role name, so
-- a new role needs no new column. A case has at most one party per role; assigning a role
-- again replaces its holder, and deleting the case deletes its parties. See internal/caseparty.
CREATE TABLE IF NOT EXISTS case_parties (
	case_id text NOT NULL REFERENCES cases (id) ON DELETE CASCADE,
	role text NOT NULL,
	party_type text NOT NULL CONSTRAINT case_parties_party_type_check CHECK (party_type IN ('company', 'user')),
	party_id text NOT NULL,
	assigned_by text,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	PRIMARY KEY (case_id, role)
);

CREATE INDEX IF NOT EXISTS idx_case_parties_party ON case_parties (party_id, role);

-- created_by is the user who created the case: for a consignment, its trader user.
ALTER TABLE cases ADD COLUMN IF NOT EXISTS created_by text;

-- The trader and CHA columns of consignments stay, still written, until a later migration
-- drops them; reads come from here.
INSERT INTO case_parties (case_id, role, party_type, party_id, assigned_by, created_at, updated_at)
SELECT id, 'trader', 'company', trader_company_id, trader_id, created_at, created_at
FROM consignments
ON CONFLICT (case_id, role) DO NOTHING;

INSERT INTO case_parties (case_id, role, party_type, party_id, created_at, updated_at)
SELECT id, 'cha', 'company', cha_company_id, updated_at, updated_at
FROM consignments
WHERE cha_company_id IS NOT NULL
ON CONFLICT (case_id, role) DO NOTHING;

UPDATE cases SET created_by = consignments.trader_id
FROM consignments
WHERE consignments.id = cases.id AND cases.created_by IS NULL;

-- @DOWN
ALTER TABLE cases DROP COLUMN IF EXISTS created_by;
DROP TABLE IF EXISTS case_parties;
