-- @UP
-- ============================================================================
-- Migration: 000008_add_company_seed.sql
-- Purpose: Seed company information for different customs house agents.
-- ============================================================================

-- Seed additional companies for CDA, FCAU, NPQS, GovTech, and PIU
INSERT INTO company_records (id, name, ou_handle, has_cha, data) VALUES
    ('cda',   'CDA Company',   'cda-company',   true, '{"br_no": "PV-00123456", "vat_no": "VAT-409123456-7000", "tin_no": "134256789"}'),
    ('fcau', 'FCAU Company', 'fcau-company', true, '{"br_no": "PV-00876543", "vat_no": "VAT-409876543-7000", "tin_no": "987654321"}'),
    ('npqs', 'NPQS Company', 'npqs-company', true, '{"br_no": "PV-00000001", "vat_no": "VAT-409000000-7000", "tin_no": "000000001"}'),
    ('govtech', 'GovTech Company', 'govtech-company', true, '{"br_no": "PV-00000002", "vat_no": "VAT-409000001-7000", "tin_no": "000000002"}'),
    ('piu', 'PIU Company', 'piu-company', true, '{"br_no": "PV-00000004", "vat_no": "VAT-409000003-7000", "tin_no": "000000004"}')
ON CONFLICT (id) DO NOTHING;
