-- Created at: 2026-08-26T09:00:00Z

-- @UP
-- SLPA's Cargo Management System reports a service order's approvals by webhook,
-- and the only thing tying a decision to a consignment is the slug the CMS issued
-- when the order was raised. Every call therefore looks a task up by it.
--
-- Indexed on the expression the lookup uses, as the CDN edge id already is: the
-- slug lives inside the task's JSONB data rather than in a column of its own,
-- because it belongs to one agency's step and not to every task in the system.
CREATE INDEX IF NOT EXISTS idx_task_records_v2_slpa_order_slug
    ON task_records_v2 ((data -> 'so' ->> 'slug'));

-- @DOWN
DROP INDEX IF EXISTS idx_task_records_v2_slpa_order_slug;
