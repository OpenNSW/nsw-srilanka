-- @UP
-- The v1.0.0 schema: migrations 000001–000038 of v0.x folded into one, with the
-- tables of the retired workflow engine (workflows, workflow_nodes,
-- workflow_node_templates) left out. It installs into an empty database only.

-- ---------------------------------------------------------------------------
-- Profiles
-- ---------------------------------------------------------------------------
CREATE TABLE company_records (
	id varchar(100) NOT NULL PRIMARY KEY,
	name varchar(255) NOT NULL,
	ou_handle varchar(255) NOT NULL UNIQUE,
	has_cha boolean DEFAULT false NOT NULL,
	data jsonb DEFAULT '{}'::jsonb NOT NULL,
	created_at timestamp with time zone DEFAULT now(),
	updated_at timestamp with time zone DEFAULT now()
);

CREATE TABLE customs_house_agents (
	id varchar(100) NOT NULL PRIMARY KEY,
	name varchar(255) NOT NULL,
	description text,
	email varchar(255),
	company_id varchar(100) NOT NULL REFERENCES company_records (id),
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX idx_customs_house_agents_company_id ON customs_house_agents (company_id);

CREATE TABLE user_records (
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
CREATE INDEX idx_user_records_ou_handle ON user_records (ou_handle);

-- ---------------------------------------------------------------------------
-- Cases: the generic grouping of the workflows that work on one matter. Trade
-- detail (consignments), the flows started on a case (case_flows) and who may
-- work it (case_parties) join on the case id. See docs/agency.md.
-- ---------------------------------------------------------------------------
CREATE TABLE cases (
	id text NOT NULL PRIMARY KEY,
	name varchar(255),
	state varchar(50) DEFAULT 'IN_PROGRESS' NOT NULL
		CONSTRAINT cases_state_check CHECK (state IN ('IN_PROGRESS', 'FINISHED')),
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	created_by text,
	attributes jsonb DEFAULT '{}'::jsonb NOT NULL
		CONSTRAINT cases_attributes_check CHECK (jsonb_typeof(attributes) = 'object')
);
CREATE INDEX idx_cases_created_at ON cases (created_at DESC);
CREATE INDEX idx_cases_state ON cases (state);

CREATE TABLE case_flows (
	workflow_id text NOT NULL PRIMARY KEY,
	case_id text NOT NULL REFERENCES cases (id),
	template_id text NOT NULL,
	status text NOT NULL
		CONSTRAINT case_flows_status_check CHECK (status IN ('STARTING', 'STARTED', 'COMPLETED')),
	payload jsonb,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	manifest_version integer DEFAULT 0 NOT NULL
);
CREATE INDEX idx_case_flows_case_id ON case_flows (case_id);

CREATE TABLE case_parties (
	case_id text NOT NULL REFERENCES cases (id) ON DELETE CASCADE,
	role text NOT NULL,
	party_type text NOT NULL
		CONSTRAINT case_parties_party_type_check CHECK (party_type IN ('ou', 'user')),
	party_id text NOT NULL,
	assigned_by text,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	PRIMARY KEY (case_id, role)
);
CREATE INDEX idx_case_parties_party ON case_parties (party_id, role);

-- A consignment is a case with trade detail: its id is the case's id.
CREATE TABLE consignments (
	id text NOT NULL PRIMARY KEY REFERENCES cases (id),
	flow varchar(50) NOT NULL
		CONSTRAINT consignments_flow_check CHECK (flow IN ('IMPORT', 'EXPORT')),
	trader_id varchar(100) NOT NULL,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	trader_company_id varchar(100) NOT NULL REFERENCES company_records (id),
	cha_company_id varchar(100) REFERENCES company_records (id),
	cha_id varchar(100) REFERENCES customs_house_agents (id)
);
CREATE INDEX idx_consignments_cha_company_id ON consignments (cha_company_id);
CREATE INDEX idx_consignments_cha_id ON consignments (cha_id);
CREATE INDEX idx_consignments_created_at ON consignments (created_at DESC);
CREATE INDEX idx_consignments_flow ON consignments (flow);
CREATE INDEX idx_consignments_trader_company_id ON consignments (trader_company_id);
CREATE INDEX idx_consignments_trader_id ON consignments (trader_id);

-- ---------------------------------------------------------------------------
-- Tasks: github.com/OpenNSW/core/taskflow names these tables
-- (gormstore.TaskRecordModel, TaskRoleModel).
-- ---------------------------------------------------------------------------
CREATE TABLE taskflow_tasks (
	task_id text NOT NULL PRIMARY KEY,
	task_type text,
	state text,
	parent_workflow_id text,
	task_workflow_id text,
	active_task_template_id text,
	case_id text DEFAULT '' NOT NULL,
	data jsonb,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	parent_step_id text,
	active_step_id uuid,
	seq bigint DEFAULT 0 NOT NULL,
	claimed_by text,
	claimed_at timestamp with time zone,
	manifest_version integer DEFAULT 0 NOT NULL,
	task_template_id text DEFAULT '' NOT NULL
);
CREATE INDEX idx_taskflow_tasks_case_id ON taskflow_tasks (case_id);
CREATE INDEX idx_taskflow_tasks_parent_workflow_id ON taskflow_tasks (parent_workflow_id);
CREATE INDEX idx_taskflow_tasks_task_workflow_id ON taskflow_tasks (task_workflow_id);
-- SLPA webhooks find a task by its service order.
CREATE INDEX idx_taskflow_tasks_slpa_order_slug ON taskflow_tasks (((data -> 'so') ->> 'slug'));
CREATE INDEX idx_taskflow_tasks_slpa_service_order_no ON taskflow_tasks (((data -> 'so') ->> 'service_order_no'));

-- One row per read role of a task, written with the task. case_id is copied
-- from the task, so a case's roles match without reading taskflow_tasks.
CREATE TABLE taskflow_task_roles (
	task_id text NOT NULL,
	role text NOT NULL,
	case_id text DEFAULT '' NOT NULL,
	PRIMARY KEY (task_id, role)
);
CREATE INDEX idx_taskflow_task_roles_case_role ON taskflow_task_roles (case_id, role);
CREATE INDEX idx_taskflow_task_roles_role ON taskflow_task_roles (role);

-- ---------------------------------------------------------------------------
-- Reference IDs: github.com/OpenNSW/core/refid's default table names.
-- ---------------------------------------------------------------------------
CREATE TABLE refid_sequences (
	scope_key text NOT NULL PRIMARY KEY,
	counter bigint DEFAULT 0 NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE refid_random_values (
	scope_key text NOT NULL,
	value text NOT NULL,
	issued_at timestamp with time zone DEFAULT now() NOT NULL,
	PRIMARY KEY (scope_key, value)
);

-- ---------------------------------------------------------------------------
-- Payments: github.com/OpenNSW/core/payment's Transaction.
-- ---------------------------------------------------------------------------
CREATE TABLE payment_transactions (
	id text NOT NULL PRIMARY KEY,
	reference_number varchar(255) NOT NULL UNIQUE,
	session_id varchar(255),
	amount numeric(15,2) NOT NULL,
	currency varchar(10) NOT NULL,
	status varchar(50) NOT NULL,
	payment_method varchar(50),
	gateway_id varchar(255) DEFAULT '' NOT NULL,
	expiry_date timestamp with time zone NOT NULL,
	gateway_metadata jsonb,
	created_at timestamp with time zone DEFAULT now(),
	updated_at timestamp with time zone DEFAULT now(),
	callback_token text DEFAULT '' NOT NULL,
	org_id text DEFAULT '' NOT NULL,
	service_id text DEFAULT '' NOT NULL
);
CREATE INDEX idx_payment_tx_payee ON payment_transactions (org_id, service_id);

-- ---------------------------------------------------------------------------
-- ASYCUDA via SLC Edge: each declaration and dispatch note records the case and
-- workflow it was dispatched from, so a callback finds the waiting task.
-- ---------------------------------------------------------------------------
CREATE TABLE cusdec_declarations (
	id text NOT NULL PRIMARY KEY,
	edge_id text NOT NULL,
	status text NOT NULL,
	cusdec_year text,
	cusdec_office text,
	cusdec_serial text,
	cusdec_number integer,
	errors jsonb,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	case_id text,
	workflow_id text
);
CREATE UNIQUE INDEX idx_cusdec_declarations_edge_id ON cusdec_declarations (edge_id);
CREATE INDEX idx_cusdec_declarations_status ON cusdec_declarations (status);
CREATE INDEX idx_cusdec_ref ON cusdec_declarations (cusdec_year, cusdec_office, cusdec_serial, cusdec_number);

CREATE TABLE dispatch_notes (
	id text NOT NULL PRIMARY KEY,
	edge_id text NOT NULL,
	status text NOT NULL,
	cdn_year text,
	cdn_office text,
	cdn_serial text,
	cdn_number integer,
	errors jsonb,
	created_at timestamp with time zone DEFAULT now() NOT NULL,
	updated_at timestamp with time zone DEFAULT now() NOT NULL,
	case_id text,
	workflow_id text
);
CREATE UNIQUE INDEX idx_dispatch_notes_edge_id ON dispatch_notes (edge_id);
CREATE INDEX idx_dispatch_notes_status ON dispatch_notes (status);
CREATE INDEX idx_cdn_ref ON dispatch_notes (cdn_year, cdn_office, cdn_serial, cdn_number);

-- @DOWN
DROP TABLE IF EXISTS dispatch_notes;
DROP TABLE IF EXISTS cusdec_declarations;
DROP TABLE IF EXISTS payment_transactions;
DROP TABLE IF EXISTS refid_random_values;
DROP TABLE IF EXISTS refid_sequences;
DROP TABLE IF EXISTS taskflow_task_roles;
DROP TABLE IF EXISTS taskflow_tasks;
DROP TABLE IF EXISTS consignments;
DROP TABLE IF EXISTS case_parties;
DROP TABLE IF EXISTS case_flows;
DROP TABLE IF EXISTS cases;
DROP TABLE IF EXISTS user_records;
DROP TABLE IF EXISTS customs_house_agents;
DROP TABLE IF EXISTS company_records;
