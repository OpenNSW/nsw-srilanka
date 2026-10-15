-- Created at: 2026-10-12T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- The name shown for a user, e.g. as a task's claim holder: their token's given and
-- family names, stored at login and refreshed when they change. Empty until a token
-- carries a name; existing users get theirs on their next login.
ALTER TABLE user_records ADD COLUMN IF NOT EXISTS display_name VARCHAR(255) NOT NULL DEFAULT '';

-- @DOWN
ALTER TABLE user_records DROP COLUMN IF EXISTS display_name;
