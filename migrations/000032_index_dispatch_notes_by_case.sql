-- Created at: 2026-10-27T17:50:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- An amended declaration reconciles a consignment's dispatch note branches with
-- the containers it now has, reading the notes the consignment raised by case.
CREATE INDEX IF NOT EXISTS idx_dispatch_notes_case_id ON dispatch_notes (case_id);

-- @DOWN
DROP INDEX IF EXISTS idx_dispatch_notes_case_id;
