-- Created at: 2026-09-30T07:25:15Z

-- @UP
-- Durable state for github.com/OpenNSW/core/refid, which the REFID_GENERATOR
-- task plugin generates reference IDs through. The DDL matches refid's
-- store/postgres MigrateSequence and MigrateRandom.

-- One counter per resolved scope key: a sequence segment's next number.
CREATE TABLE IF NOT EXISTS refid_sequences (
    scope_key  TEXT        NOT NULL PRIMARY KEY,
    counter    BIGINT      NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Every value a random segment has issued, per scope key, so a repeat is
-- detected and redrawn.
CREATE TABLE IF NOT EXISTS refid_random (
    scope_key  TEXT        NOT NULL,
    value      TEXT        NOT NULL,
    issued_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope_key, value)
);

-- @DOWN
DROP TABLE IF EXISTS refid_random;
DROP TABLE IF EXISTS refid_sequences;
