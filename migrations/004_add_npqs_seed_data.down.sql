-- ============================================================================
-- Migration: 004_add_npqs_seed_data.down.sql
-- Purpose: Delete NPQS top-level workflow template, HS code, and mapping.
-- ============================================================================

DELETE FROM workflow_template_map WHERE id = 'npqs-wf-map-0001';
DELETE FROM workflow_template_v2 WHERE id = 'npqs-export-phytosanitary-reg';
DELETE FROM hs_codes WHERE id = 'npqs-hs-code-0001';
