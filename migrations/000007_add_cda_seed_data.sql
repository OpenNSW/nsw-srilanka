-- @UP
-- ============================================================================
-- Migration: 000007_add_cda_seed_data.sql
-- Purpose: Seed CDA top-level workflow template, HS code, and mapping.
-- ============================================================================

-- 1. Seed the HS code for CDA
INSERT INTO hs_codes (id, hs_code, description, category)
VALUES 
    (
        'cda-hs-code-0001',
        'cda-coconut-certificate',
        'HS code for the CDA coconut certificate registration flow.',
        'CDA'
    )
ON CONFLICT (id) DO NOTHING;

-- 2. Seed mapping for CDA HS code to workflow template
-- Note: cda-certificate-reg is loaded dynamically from configs/cda/cda_workflow.json on startup
-- and persisted to workflow_template_v2 by the app engine before the foreign key constraint is established.
INSERT INTO workflow_template_map (id, hs_code_id, consignment_flow, workflow_template_id)
VALUES 
    (
        'cda-wf-map-0001',
        'cda-hs-code-0001',
        'EXPORT',
        'cda-certificate-reg'
    )
ON CONFLICT (id) DO NOTHING;

-- @DOWN
-- ============================================================================
-- Migration: 000007_add_cda_seed_data.sql
-- Purpose: Rollback CDA seed data mapping and HS code.
-- ============================================================================

DELETE FROM workflow_template_map WHERE id = 'cda-wf-map-0001';
DELETE FROM hs_codes WHERE id = 'cda-hs-code-0001';
