-- Created at: 2026-10-11T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- case_flows records every top-level workflow (a flow) started on a case: the export
-- flow, an auxiliary flow a trader starts, a notice. A case is FINISHED once every one of
-- its flows is COMPLETED, and a new flow reopens it. workflow_id is the key, so a repeated
-- start of the same flow is a no-op rather than a second run. payload holds the variables
-- the flow was first started with, so a retried start runs with them. See
-- internal/caseflow.
CREATE TABLE IF NOT EXISTS case_flows (
	workflow_id text NOT NULL PRIMARY KEY,
	case_id text NOT NULL REFERENCES cases (id),
	template_id text NOT NULL,
	status text NOT NULL CONSTRAINT case_flows_status_check CHECK (status IN ('STARTING', 'STARTED', 'COMPLETED')),
	payload jsonb,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_case_flows_case_id ON case_flows (case_id);

-- @DOWN
DROP TABLE IF EXISTS case_flows;
