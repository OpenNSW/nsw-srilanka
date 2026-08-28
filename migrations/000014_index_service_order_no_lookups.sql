-- Created at: 2026-08-26T11:00:00Z

-- @UP
-- SLPA's invoice webhooks identify an order by its slug or, when that is absent,
-- by the order number the CMS issued. The slug is already indexed (000013); this
-- covers the other half of that lookup so neither path scans the table.
CREATE INDEX IF NOT EXISTS idx_task_records_v2_slpa_service_order_no
    ON task_records_v2 ((data -> 'so' ->> 'service_order_no'));

-- @DOWN
DROP INDEX IF EXISTS idx_task_records_v2_slpa_service_order_no;
