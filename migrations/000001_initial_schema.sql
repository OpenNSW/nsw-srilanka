-- @UP
-- ============================================================================
-- Migration: 000001_initial_schema.sql
-- Purpose: Create baseline schema tables, constraints, indexes.
-- ============================================================================

-- -------------------------------------------------------------------
-- 1. Base Business Profiles
-- -------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS company_records (
	id varchar(100) NOT NULL PRIMARY KEY,
	name varchar(255) NOT NULL,
	ou_handle varchar(255) NOT NULL UNIQUE,
	has_cha boolean NOT NULL DEFAULT false,
	data jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp with time zone DEFAULT now(),
	updated_at timestamp with time zone DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_company_records_ou_handle ON company_records (ou_handle);

CREATE TABLE IF NOT EXISTS customs_house_agents (
	id varchar(100) NOT NULL PRIMARY KEY,
	name varchar(255) NOT NULL,
	description text,
	email varchar(255),
	company_id varchar(100) NOT NULL REFERENCES company_records (id),
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_customs_house_agents_company_id ON customs_house_agents (company_id);

CREATE TABLE IF NOT EXISTS user_records (
	id varchar(100) NOT NULL PRIMARY KEY,
	idp_user_id varchar(255) NOT NULL UNIQUE,
	email varchar(255) NOT NULL,
	phone_number varchar(20),
	ou_id varchar(255) NOT NULL,
	ou_handle varchar(255) NOT NULL,
	data jsonb NOT NULL,
	created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
	updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_records_ou_handle ON user_records (ou_handle);
CREATE INDEX IF NOT EXISTS idx_user_records_idp_user_id ON user_records (idp_user_id);

-- -------------------------------------------------------------------
-- 2. HS Codes & Workflow Mappings
-- -------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS hs_codes (
	id text NOT NULL PRIMARY KEY,
	hs_code varchar(50) NOT NULL UNIQUE,
	description text NOT NULL,
	category varchar(100),
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hs_codes_hs_code ON hs_codes (hs_code);

CREATE TABLE IF NOT EXISTS workflow_template_map (
	id text NOT NULL PRIMARY KEY,
	hs_code_id text NOT NULL REFERENCES hs_codes(id) ON UPDATE CASCADE ON DELETE RESTRICT,
	consignment_flow varchar(50) NOT NULL
		CONSTRAINT workflow_template_map_consignment_flow_check
			CHECK ((consignment_flow)::text = ANY ((ARRAY['IMPORT'::character varying, 'EXPORT'::character varying])::text[])),
	workflow_template_id text NOT NULL,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);
-- Index the hs_code_id FK: workflow-template selection joins on it, so without
-- this Postgres would fall back to a sequential scan.
CREATE INDEX IF NOT EXISTS idx_workflow_template_map_hs_code_id ON workflow_template_map (hs_code_id);

-- -------------------------------------------------------------------
-- 3. Workflow Instances & Node States
-- -------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflows (
    id text NOT NULL PRIMARY KEY,
    status varchar(50) NOT NULL DEFAULT 'IN_PROGRESS'
        CONSTRAINT workflows_status_check
            CHECK ((status)::text = ANY ((ARRAY['IN_PROGRESS'::character varying, 'COMPLETED'::character varying, 'FAILED'::character varying])::text[])),
    global_context jsonb NOT NULL DEFAULT '{}'::jsonb,
    end_node_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workflows_status ON workflows (status);

CREATE TABLE IF NOT EXISTS workflow_node_templates (
	id text NOT NULL PRIMARY KEY,
	name varchar(255) NOT NULL,
	description text,
	type varchar(50) NOT NULL,
	config jsonb NOT NULL,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workflow_node_templates_name ON workflow_node_templates (name);
CREATE INDEX IF NOT EXISTS idx_workflow_node_templates_type ON workflow_node_templates (type);
CREATE INDEX IF NOT EXISTS idx_workflow_node_templates_config ON workflow_node_templates USING gin (config);

CREATE TABLE IF NOT EXISTS workflow_nodes (
	id text NOT NULL PRIMARY KEY,
	workflow_id text NOT NULL REFERENCES workflows(id) ON UPDATE CASCADE ON DELETE CASCADE,
	workflow_node_template_id text NOT NULL REFERENCES workflow_node_templates(id) ON UPDATE CASCADE ON DELETE RESTRICT,
	state varchar(50) NOT NULL CONSTRAINT workflow_nodes_state_check CHECK ((state)::text = ANY ((ARRAY['LOCKED'::character varying, 'READY'::character varying, 'IN_PROGRESS'::character varying, 'COMPLETED'::character varying, 'FAILED'::character varying])::text[])),
	extended_state text,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	outcome varchar(100)
);
CREATE INDEX IF NOT EXISTS idx_workflow_nodes_workflow_id ON workflow_nodes (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_nodes_workflow_id_state ON workflow_nodes (workflow_id, state);
CREATE INDEX IF NOT EXISTS idx_workflow_nodes_workflow_node_template_id ON workflow_nodes (workflow_node_template_id);
CREATE INDEX IF NOT EXISTS idx_workflow_nodes_state ON workflow_nodes (state);

-- -------------------------------------------------------------------
-- 4. Business Consignments
--    cha_company_id is nullable: direct-start consignments (e.g.
--    trade-export-v1) select the CHA inside the workflow, not up front.
-- -------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS consignments (
	id text NOT NULL PRIMARY KEY,
	flow varchar(50) NOT NULL CONSTRAINT consignments_flow_check CHECK ((flow)::text = ANY ((ARRAY['IMPORT'::character varying, 'EXPORT'::character varying])::text[])),
	trader_id varchar(100) NOT NULL,
	state varchar(50) NOT NULL CONSTRAINT consignments_state_check CHECK ((state)::text = ANY (ARRAY['INITIALIZED'::character varying, 'IN_PROGRESS'::character varying, 'FINISHED'::character varying])),
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	trader_company_id varchar(100) NOT NULL REFERENCES company_records (id),
	cha_company_id varchar(100) REFERENCES company_records (id),
	cha_id varchar(100) REFERENCES customs_house_agents (id)
);
CREATE INDEX IF NOT EXISTS idx_consignments_trader_id ON consignments (trader_id);
CREATE INDEX IF NOT EXISTS idx_consignments_trader_company_id ON consignments (trader_company_id);
CREATE INDEX IF NOT EXISTS idx_consignments_cha_company_id ON consignments (cha_company_id);
CREATE INDEX IF NOT EXISTS idx_consignments_state ON consignments (state);
CREATE INDEX IF NOT EXISTS idx_consignments_flow ON consignments (flow);
CREATE INDEX IF NOT EXISTS idx_consignments_created_at ON consignments (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_consignments_cha_id ON consignments (cha_id);

-- -------------------------------------------------------------------
-- 5. Taskv2 & Payment Tables
-- -------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS payment_transactions (
    id text NOT NULL PRIMARY KEY,
    reference_number VARCHAR(255) UNIQUE NOT NULL,
    task_id VARCHAR(255) NOT NULL,
    session_id VARCHAR(255),
    amount NUMERIC(15, 2) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    status VARCHAR(50) NOT NULL,
    payment_method VARCHAR(50),
    gateway_id VARCHAR(255) NOT NULL DEFAULT '',
    expiry_date TIMESTAMP WITH TIME ZONE NOT NULL,
    gateway_metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_payment_tx_task_id ON payment_transactions (task_id);
CREATE INDEX idx_payment_tx_reference_number ON payment_transactions (reference_number);

CREATE TABLE IF NOT EXISTS task_records_v2 (
    task_id                 TEXT PRIMARY KEY,
    task_type               TEXT,
    state                   TEXT,
    render_config           JSONB,
    parent_workflow_id      TEXT,
    parent_run_id           TEXT,
    parent_node_id          TEXT,
    task_workflow_id        TEXT,
    task_run_id             TEXT,
    subtask_node_id         TEXT,
    active_task_template_id TEXT,
    active_output_namespace TEXT NOT NULL DEFAULT '',
    root_workflow_id        TEXT NOT NULL DEFAULT '',
    data                    JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_task_records_v2_parent_workflow_id ON task_records_v2(parent_workflow_id);
CREATE INDEX idx_task_records_v2_task_workflow_id ON task_records_v2(task_workflow_id);
CREATE INDEX idx_task_records_v2_root_workflow_id ON task_records_v2(root_workflow_id);

-- @DOWN
-- ============================================================================
-- Roll back baseline schema objects in reverse dependency order.
-- ============================================================================

-- 1. Drop runtime workflow nodes and consignments first due to foreign keys
DROP TABLE IF EXISTS workflow_nodes;
DROP TABLE IF EXISTS consignments;

-- 2. Drop dependent schema tables
DROP TABLE IF EXISTS customs_house_agents;
DROP TABLE IF EXISTS workflow_template_map;
DROP TABLE IF EXISTS company_records;
DROP TABLE IF EXISTS hs_codes;
DROP TABLE IF EXISTS user_records;
DROP TABLE IF EXISTS workflows;

-- 3. Drop legacy / engine / config tables
DROP TABLE IF EXISTS task_records_v2;
DROP TABLE IF EXISTS payment_transactions;
DROP TABLE IF EXISTS workflow_node_templates;