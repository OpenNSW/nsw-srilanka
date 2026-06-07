-- Direct-start consignments (e.g. trade-export-v1) no longer pick a CHA company up front —
-- CHA selection now happens inside the workflow itself (trade_1_cha_selection task), so
-- cha_company_id is not known at consignment-creation time and must be nullable.
ALTER TABLE consignments ALTER COLUMN cha_company_id DROP NOT NULL;
