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
-- ADAM PVT LTD gets the key SLPA actually issued for the integration; every
-- other company gets a generated one of the same shape (10 alphanumerics) so a
-- local environment can exercise the flow as any company. These are development
-- values: a real deployment overwrites them with the keys SLPA issued.
UPDATE company_records
SET data = COALESCE(data, '{}'::jsonb) || jsonb_build_object('slpacmsuser_key', 'UkLWZg9DAJ'),
    updated_at = now()
WHERE id = 'adam-pvt-ltd';

UPDATE company_records
SET data = COALESCE(data, '{}'::jsonb) || jsonb_build_object(
        'slpacmsuser_key',
        -- 10 characters drawn from the same alphabet as the issued key, seeded
        -- from the company id so re-running is idempotent.
        upper(substr(md5(id || 'slpa-cms'), 1, 5)) || substr(md5(id), 6, 5)
    ),
    updated_at = now()
WHERE NOT (data ? 'slpacmsuser_key');

-- @DOWN
UPDATE company_records
SET data = data - 'slpacmsuser_key',
    updated_at = now()
WHERE data ? 'slpacmsuser_key';
