-- ============================================================================
-- Migration: 004_add_npqs_seed_data.up.sql
-- Purpose: Seed NPQS top-level workflow template, HS code, and mapping.
-- ============================================================================

INSERT INTO workflow_template_v2 (id, name, version, workflow_definition)
VALUES 
(
    'npqs-export-phytosanitary-reg',
    'NPQS Export Consignment & Phytosanitary Registration',
    '3',
    '{}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

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
