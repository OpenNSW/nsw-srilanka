-- Created at: 2026-08-20T09:00:00Z

-- @UP
-- SLPA's Cargo Management System identifies an ECDN upload by the client key it
-- issued to the submitting company (the CMS calls it the ClientSqid, sent as the
-- slpacmsuser-key header). It is per-company, not per-deployment, so it belongs
-- on the company profile, from where the workflow carries it to the SLPA task.
--
-- Stored in the existing data JSONB rather than a new column: it is one optional
-- identifier among the others already there (br_no, tin_no, vat_no), and only
-- companies registered with SLPA have one.
--
-- Every company gets the one key SLPA issued for this integration, so a local
-- environment can exercise the flow as whichever company it signs in as. That is
-- a development convenience, not the real shape: SLPA issues a key per
-- registered company, and a real deployment overwrites these with the key each
-- company was issued — companies not registered with SLPA carry none at all, and
-- their submissions are refused by the CMS.
UPDATE company_records
SET data = COALESCE(data, '{}'::jsonb) || jsonb_build_object('slpacmsuser_key', 'agztNvLSUA'),
    updated_at = now();

-- @DOWN
UPDATE company_records
SET data = data - 'slpacmsuser_key',
    updated_at = now()
WHERE data ? 'slpacmsuser_key';
