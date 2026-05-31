-- ============================================================================
-- Migration: 003_insert_seed_data.down.sql
-- Purpose: Roll back baseline seeded companies, CHAs, and workflow mappings.
-- ============================================================================

DELETE FROM workflow_template_map WHERE id IN ('c3d4e5f6-0001-4000-d000-000000000001', 'fcau-wf-map-0001', 'fcau-wf-map-0002');
DELETE FROM workflow_template_v2 WHERE id IN ('trade-export-v1', 'fcau-health-certificate-reg');
DELETE FROM customs_house_agents WHERE id IN ('a1b2c3d4-0001-4000-8000-000000000001', 'a1b2c3d4-0002-4000-8000-000000000002', 'a1b2c3d4-0003-4000-8000-000000000003');
DELETE FROM company_records WHERE id IN ('adam-pvt-ltd', 'edward-pvt-ltd');
DELETE FROM hs_codes WHERE id IN ('fcau-hs-code-0001', 'fcau-hs-code-0002');
