-- ============================================================================
-- Migration: 005_add_cda_seed_data.down.sql
-- Purpose: Rollback CDA seed data mapping and HS code.
-- ============================================================================

DELETE FROM workflow_template_map WHERE id = 'cda-wf-map-0001';
DELETE FROM hs_codes WHERE id = 'cda-hs-code-0001';
