-- Created at: 2026-10-07T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- core/payment completes a settled payment's workflow step by the opaque callback
-- token the checkout carried, and no longer knows which task a payment is for:
-- it stores the token and stops writing task_id. The payment plugin still puts
-- the task ID in the checkout metadata (gateway_metadata.task_id) for
-- correlation.
ALTER TABLE payment_transactions ADD COLUMN IF NOT EXISTS callback_token TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_payment_tx_task_id;
ALTER TABLE payment_transactions DROP COLUMN IF EXISTS task_id;

-- @DOWN
ALTER TABLE payment_transactions ADD COLUMN IF NOT EXISTS task_id VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_payment_tx_task_id ON payment_transactions (task_id);

ALTER TABLE payment_transactions DROP COLUMN IF EXISTS callback_token;
