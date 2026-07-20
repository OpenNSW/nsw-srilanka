-- Created at: 2026-07-20T07:03:00Z

-- @UP
CREATE TABLE IF NOT EXISTS dispatch_notes (
	id text NOT NULL PRIMARY KEY,
	edg_id text NOT NULL,
	status text NOT NULL,
	cdn_year text,
	cdn_office text,
	cdn_serial text,
	cdn_number integer,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dispatch_notes_edg_id ON dispatch_notes (edg_id);
CREATE INDEX IF NOT EXISTS idx_dispatch_notes_status ON dispatch_notes (status);
CREATE INDEX IF NOT EXISTS idx_cdn_ref ON dispatch_notes (cdn_year, cdn_office, cdn_serial, cdn_number);

-- @DOWN
DROP TABLE IF EXISTS dispatch_notes;
