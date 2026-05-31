-- ============================================================================
-- Migration: 001_initial_schema.down.sql
-- Purpose: Roll back baseline schema objects in reverse dependency order.
-- ============================================================================

-- 1. Drop runtime workflow nodes and consignments first due to foreign keys
DROP TABLE IF EXISTS workflow_nodes;
DROP TABLE IF EXISTS consignments;

-- 2. Drop dependent schema tables
DROP TABLE IF EXISTS customs_house_agents;
DROP TABLE IF EXISTS company_records;
DROP TABLE IF EXISTS workflow_template_map;
DROP TABLE IF EXISTS workflow_template_v2;
DROP TABLE IF EXISTS hs_codes;
DROP TABLE IF EXISTS user_records;
DROP TABLE IF EXISTS workflows;

-- 3. Drop legacy / engine / config tables
DROP TABLE IF EXISTS task_records_v2;
DROP TABLE IF EXISTS task_workflow_tasks;
DROP TABLE IF EXISTS payment_transactions;
DROP TABLE IF EXISTS forms;
DROP TABLE IF EXISTS task_infos;
DROP TABLE IF EXISTS workflow_node_templates;
DROP TABLE IF EXISTS user_contexts;
