-- Created at: 2026-09-28T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- cases is the generic grouping of the workflows that work on one real-world matter
-- (for an agency, the consignment TNSW injects under). It carries only the columns of
-- consignments that are not trade-specific: flow, trader and CHA detail belong in their
-- own table joined on case id. See docs/agency.md.
CREATE TABLE IF NOT EXISTS cases (
	id text NOT NULL PRIMARY KEY,
	name varchar(255),
	state varchar(50) DEFAULT 'IN_PROGRESS' NOT NULL CONSTRAINT cases_state_check CHECK ((state)::text = ANY (ARRAY['IN_PROGRESS'::character varying, 'FINISHED'::character varying])),
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cases_state ON cases (state);
CREATE INDEX IF NOT EXISTS idx_cases_created_at ON cases (created_at DESC);

CREATE TABLE IF NOT EXISTS agency_workflow (
	task_id text NOT NULL PRIMARY KEY,
	task_code text NOT NULL,
	case_id text NOT NULL REFERENCES cases (id),
	status text NOT NULL,
	payload jsonb,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_workflow_case_id ON agency_workflow (case_id);

-- @DOWN
DROP TABLE IF EXISTS agency_workflow;
DROP TABLE IF EXISTS cases;
