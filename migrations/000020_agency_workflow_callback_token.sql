-- Created at: 2026-10-06T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- The opaque token the injecting system expects its decision back on
-- (POST /api/v1/callbacks/{callbackToken} on TNSW). It is recorded with the row, like the
-- payload, so a retried start seeds the workflow with the token of the first inject.
-- NULL when the inject carried no token.
ALTER TABLE agency_workflow ADD COLUMN IF NOT EXISTS callback_token TEXT;

-- @DOWN
ALTER TABLE agency_workflow DROP COLUMN IF EXISTS callback_token;
