-- Created at: 2026-11-03T16:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- An agency's flows are recorded in case_flows, like TNSW's (see internal/caseflow), so
-- agency_workflow goes. Its rows are dropped, not moved: an injected workflow still running
-- is no recorded flow, so its completion callback fails and retries. End them before this.
DROP TABLE IF EXISTS agency_workflow;

-- @DOWN
-- The table comes back empty, as migrations 18, 20 and 31 left it.
CREATE TABLE IF NOT EXISTS agency_workflow (
	task_id text NOT NULL PRIMARY KEY,
	task_code text NOT NULL,
	case_id text NOT NULL REFERENCES cases (id),
	status text NOT NULL,
	payload jsonb,
	callback_token text,
	manifest_version integer NOT NULL DEFAULT 0,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_workflow_case_id ON agency_workflow (case_id);
