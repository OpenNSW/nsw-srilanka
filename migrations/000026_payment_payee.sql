-- Created at: 2026-10-21T00:00:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- core/payment v0.4.0 records the payee a transaction pays, as org_id and
-- service_id, and resolves the gateway's ids for it from payment_methods.json
-- on every callback. Fill them in for existing transactions, so a payment still
-- pending can be presented and settled: from the GovPay+ ids the payment plugin
-- stored in gateway_metadata (govpay_subinst_id, govpay_service_id), which name
-- exactly one fee each, or else from the fee's task code (task_code), which
-- older artifacts declared. A transaction matching neither keeps empty ids and
-- can no longer be presented or settled.
ALTER TABLE payment_transactions ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_transactions ADD COLUMN IF NOT EXISTS service_id TEXT NOT NULL DEFAULT '';

UPDATE payment_transactions AS t
SET org_id = p.org_id, service_id = p.service_id
FROM (VALUES
	('001', '001', 'fcau', 'app_fee'),
	('002', '001', 'npqs', 'certificate_fee'),
	('002', '003', 'npqs', 'treatment_fee'),
	('003', '001', 'sltb', 'levy_fee'),
	('003', '002', 'sltb', 'lab_fee'),
	('004', '001', 'cda', 'app_fee'),
	('004', '002', 'cda', 'kcfn_fee')
) AS p(subinst_id, govpay_service_id, org_id, service_id)
WHERE t.gateway_metadata->>'govpay_subinst_id' = p.subinst_id
  AND t.gateway_metadata->>'govpay_service_id' = p.govpay_service_id
  AND t.org_id = '';

UPDATE payment_transactions AS t
SET org_id = p.org_id, service_id = p.service_id
FROM (VALUES
	('fcau_app_fee_payment_v1', 'fcau', 'app_fee'),
	('npqs_certificate_fee_payment', 'npqs', 'certificate_fee'),
	('npqs_treatment_payment', 'npqs', 'treatment_fee'),
	('sltb_levy_payment_v1', 'sltb', 'levy_fee'),
	('sltb_lab_payment_v1', 'sltb', 'lab_fee'),
	('cda_app_fee_payment_v1', 'cda', 'app_fee'),
	('cda_kcfn_payment_v1', 'cda', 'kcfn_fee')
) AS p(task_code, org_id, service_id)
WHERE t.gateway_metadata->>'task_code' = p.task_code
  AND t.org_id = '';

CREATE INDEX IF NOT EXISTS idx_payment_tx_payee ON payment_transactions (org_id, service_id);

-- @DOWN
DROP INDEX IF EXISTS idx_payment_tx_payee;
ALTER TABLE payment_transactions DROP COLUMN IF EXISTS service_id;
ALTER TABLE payment_transactions DROP COLUMN IF EXISTS org_id;
