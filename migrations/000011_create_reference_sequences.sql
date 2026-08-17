-- Created at: 2026-08-10T13:30:00Z

-- @UP
CREATE TABLE IF NOT EXISTS reference_sequences (
	scope_key     VARCHAR(255) PRIMARY KEY,
	current_value BIGINT NOT NULL DEFAULT 0,
	updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- @DOWN
DROP TABLE IF EXISTS reference_sequences;
