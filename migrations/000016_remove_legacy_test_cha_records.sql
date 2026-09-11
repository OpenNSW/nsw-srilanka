-- @UP
-- ============================================================================
-- Migration: 000016_remove_legacy_test_cha_records.sql
-- Purpose: Remove the three legacy test CHA records (Suresh/Ramesh/Naresh)
-- originally inserted by 000003_insert_seed_data.sql. That migration is now a
-- no-op stub, which only stops these rows from being (re)created on a fresh
-- database — it does not remove them from an environment that already ran
-- the original 000003. This migration does that removal explicitly.
--
-- adam-pvt-ltd/edward-pvt-ltd (company_records) are intentionally left alone:
-- they are still seeded locally via `otc company apply` against
-- configs/companies.example.json.
--
-- consignments.cha_id references customs_house_agents(id) with no ON DELETE
-- clause (defaults to NO ACTION), so any consignment still pointing at one of
-- these test CHAs is cleared first to avoid a foreign-key violation.
-- ============================================================================
UPDATE consignments SET cha_id = NULL
WHERE cha_id IN (
    'a1b2c3d4-0001-4000-8000-000000000001',
    'a1b2c3d4-0002-4000-8000-000000000002',
    'a1b2c3d4-0003-4000-8000-000000000003'
);

DELETE FROM customs_house_agents
WHERE id IN (
    'a1b2c3d4-0001-4000-8000-000000000001',
    'a1b2c3d4-0002-4000-8000-000000000002',
    'a1b2c3d4-0003-4000-8000-000000000003'
);
-- @DOWN
-- ============================================================================
-- Migration: 000016_remove_legacy_test_cha_records.sql
-- Purpose: Restore the three legacy test CHA records. Any consignments.cha_id
-- link that was cleared by @UP is not restored (the original association is
-- not recoverable from this migration).
-- ============================================================================
INSERT INTO customs_house_agents (id, name, description, email, company_id)
VALUES
    ('a1b2c3d4-0001-4000-8000-000000000001', 'Suresh', 'User with Trader and CHA roles at ADAM PVT LTD',   'suresh@adam-pvt-ltd.private-sector.dev',  'adam-pvt-ltd'),
    ('a1b2c3d4-0002-4000-8000-000000000002', 'Ramesh', 'User with CHA role at ADAM PVT LTD',              'ramesh@adam-pvt-ltd.private-sector.dev',  'adam-pvt-ltd'),
    ('a1b2c3d4-0003-4000-8000-000000000003', 'Naresh', 'User with CHA role at EDWARD PVT LTD',            'naresh@edward-pvt-ltd.private-sector.dev','edward-pvt-ltd')
ON CONFLICT (id) DO NOTHING;
