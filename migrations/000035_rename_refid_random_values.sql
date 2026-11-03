-- Created at: 2026-11-03T13:39:19Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- core/refid v0.5.0 names its random-segment table refid_random_values
-- (store/postgres DefaultRandomTableName). The table is renamed, with its primary key
-- index, so the values random segments have already issued are kept and a repeat is
-- still detected. refid_sequences keeps its name.
ALTER TABLE refid_random RENAME TO refid_random_values;
ALTER INDEX IF EXISTS refid_random_pkey RENAME TO refid_random_values_pkey;

-- @DOWN
ALTER INDEX IF EXISTS refid_random_values_pkey RENAME TO refid_random_pkey;
ALTER TABLE refid_random_values RENAME TO refid_random;
