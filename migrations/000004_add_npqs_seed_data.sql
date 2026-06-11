-- @UP
-- ============================================================================
-- Migration: 000004_add_npqs_seed_data.sql
-- Purpose: Seed NPQS top-level workflow template, HS code, and mapping.
-- ============================================================================

INSERT INTO hs_codes (id, hs_code, description, category)
VALUES 
    (
        'npqs-hs-code-0001',
        'npqs-phyto-certificate',
        'HS code for the NPQS phytosanitary certificate registration flow.',
        'NPQS'
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO workflow_template_map (id, hs_code_id, consignment_flow, workflow_template_id)
VALUES 
    (
        'npqs-wf-map-0001',
        'npqs-hs-code-0001',
        'EXPORT',
        'npqs-export-phytosanitary-reg'
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO hs_codes (id, hs_code, description, category)
VALUES 
    (
        'trade-hs-code-0001',
        'trade-export',
        'Flow for export trade',
        'EXPORT'
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO workflow_template_map (id, hs_code_id, consignment_flow, workflow_template_id)
VALUES 
    (
        'trade-wf-map-0001',
        'trade-hs-code-0001',
        'EXPORT',
        'trade-export-v1'
    )
ON CONFLICT (id) DO NOTHING;
-- @DOWN
-- ============================================================================
-- Migration: 000004_add_npqs_seed_data.sql
-- Purpose: Delete NPQS top-level workflow template, HS code, and mapping.
-- ============================================================================

DELETE FROM workflow_template_map WHERE id = 'npqs-wf-map-0001';
DELETE FROM hs_codes WHERE id = 'npqs-hs-code-0001';
DELETE FROM workflow_template_map WHERE id = 'trade-wf-map-0001';
DELETE FROM hs_codes WHERE id = 'trade-hs-code-0001';