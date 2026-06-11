-- @UP
-- ============================================================================
-- Migration: 000003_insert_seed_data.sql
-- Purpose: Seed baseline companies, CHAs, and top-level workflow mapping metadata.
-- ============================================================================

-- 1. Seed trader / CHA companies
INSERT INTO company_records (id, name, ou_handle, has_cha, data) VALUES
    ('adam-pvt-ltd',   'ADAM PVT LTD',   'adam-pvt-ltd',   true, '{"br_no": "PV-00123456", "vat_no": "VAT-409123456-7000", "tin_no": "134256789"}'),
    ('edward-pvt-ltd', 'EDWARD PVT LTD', 'edward-pvt-ltd', true, '{"br_no": "PV-00876543", "vat_no": "VAT-409876543-7000", "tin_no": "987654321"}')
ON CONFLICT (id) DO NOTHING;

-- 2. Seed Customs House Agents (CHAs)
INSERT INTO customs_house_agents (id, name, description, email, company_id)
VALUES
	('a1b2c3d4-0001-4000-8000-000000000001', 'Suresh', 'User with Trader and CHA roles at ADAM PVT LTD',   'suresh@adam-pvt-ltd.private-sector.dev',  'adam-pvt-ltd'),
	('a1b2c3d4-0002-4000-8000-000000000002', 'Ramesh', 'User with CHA role at ADAM PVT LTD',              'ramesh@adam-pvt-ltd.private-sector.dev',  'adam-pvt-ltd'),
	('a1b2c3d4-0003-4000-8000-000000000003', 'Naresh', 'User with CHA role at EDWARD PVT LTD',            'naresh@edward-pvt-ltd.private-sector.dev','edward-pvt-ltd')
ON CONFLICT (id) DO NOTHING;

-- 3. Seed HS code to top-level workflow mapping
-- Maps Fresh Coconut (0801.12.00 / id: 4bdfb1f0-2b71-4ddc-8b99-f31c3d7660bc) to trade-export-v1
INSERT INTO workflow_template_map (id, hs_code_id, consignment_flow, workflow_template_id)
VALUES (
    'c3d4e5f6-0001-4000-d000-000000000001',
    '4bdfb1f0-2b71-4ddc-8b99-f31c3d7660bc',
    'EXPORT',
    'trade-export-v1'
) ON CONFLICT (id) DO NOTHING;

-- 5. Seed test HS codes starting with 'f' for local testing
INSERT INTO hs_codes (id, hs_code, description, category)
VALUES 
    (
        'fcau-hs-code-0002',
        'fcau-health-certificate',
        'HS code for the FCAU health certificate registration flow.',
        'FCAU'
    )
ON CONFLICT (id) DO NOTHING;

-- 6. Seed mappings for test HS codes
INSERT INTO workflow_template_map (id, hs_code_id, consignment_flow, workflow_template_id)
VALUES 
    (
        'fcau-wf-map-0002',
        'fcau-hs-code-0002',
        'EXPORT',
        'fcau-health-certificate-reg'
    )
ON CONFLICT (id) DO NOTHING;

-- @DOWN
-- ============================================================================
-- Migration: 000003_insert_seed_data.sql
-- Purpose: Roll back baseline seeded companies, CHAs, and workflow mappings.
-- ============================================================================

DELETE FROM workflow_template_map WHERE id IN ('c3d4e5f6-0001-4000-d000-000000000001', 'fcau-wf-map-0002');
DELETE FROM customs_house_agents WHERE id IN ('a1b2c3d4-0001-4000-8000-000000000001', 'a1b2c3d4-0002-4000-8000-000000000002', 'a1b2c3d4-0003-4000-8000-000000000003');
DELETE FROM company_records WHERE id IN ('adam-pvt-ltd', 'edward-pvt-ltd');
DELETE FROM hs_codes WHERE id IN ('fcau-hs-code-0002');
