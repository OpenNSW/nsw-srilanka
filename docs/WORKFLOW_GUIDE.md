# Single-Window Workflow & Task Template Configuration Guide

This document serves as an exhaustive reference for creating, modifying, and debugging workflows, task templates, JSONForms, and rendering configurations in the NSW (National Single Window) system. Keep this guide as a direct instruction manual for any AI model (Gemini, Claude, Antigravity, etc.) tasked with writing configuration files.

---

## 1. Directory Structure: Folder-as-Task Convention

> [!IMPORTANT]
> These workflow/form artifacts are **not** committed to this application repo. They live in the public repo [OpenNSW/one-trade-artifacts](https://github.com/OpenNSW/one-trade-artifacts) under the `tnsw/` base path (`tnsw/manifest.json` + `tnsw/<agency_code>/…`) and are fetched at startup by the artifact loader (see `ARTIFACT_*` env in [`.env.example`](../.env.example)). Every path below is relative to that base path, so `<agency_code>/…` here means `tnsw/<agency_code>/…` in the artifacts repo. To edit them locally, clone that repo and point the local loader at its `tnsw` dir (`ARTIFACT_LOADER_TYPE=local`, `ARTIFACT_LOCAL_ROOT=<path>/tnsw`).

Task definitions are grouped into self-contained folders representing micro-workflows under an agency-specific folder (e.g. `<agency_code>/`). 

> [!NOTE]
> `fcau` (Food Control Administration Unit) is used as the reference example throughout this guide. For each agency process in our NSW system, we will write a similar set of configs (e.g. Coconut Development Authority (`cda`), National Plant Quarantine Service (`npqs`), etc.).

Each task folder is recognized by the `config_loader.go` registry scanner and must conform to the following file layout:

```text
<agency_code>/
├── <agency_code>_workflow.json       # Parent (top-level) workflow definition (e.g. fcau_workflow.json)
└── <task-folder>/                    # E.g. "3-1-warehouse_scheduling/" or "2-payment_app_fee/"
    ├── workflow.json                 # Micro-workflow graph definition (Required)
    ├── render.json                   # UI Zone rendering configuration (Required)
    ├── <role>input.json              # Task definition type & properties (E.g. traderinput.json, officerinput.json)
    ├── <role>input_jsonform.json     # JSONForms schema/uiSchema for interactive forms
    └── [instructions_jsonform.json]  # Optional Markdown template for static instruction boxes
```

---

## 2. Parent Workflow Definition (`<agency_code>_workflow.json`)

The parent workflow coordinates the high-level execution graph across multiple micro-workflows.

### Core Structure
- **Nodes**: List of blocks representing steps.
  - `type`: `START`, `TASK`, `GATEWAY` (split or join), or `END`.
  - `task_template_id`: Matches the `id` declared inside the child subworkflow `workflow.json`.
- **Edges**: Connectivity path mapping.
  - `condition`: String expression for conditional branching (e.g. `fcau.warehouse_inspection_required == true`).

### Critical Mapping Variable Scope Rules (Common Gotchas)
1. **Namespace isolation**: Parent variables live in the agency/process namespace (e.g. `fcau.reference_number`, `fcau.userform`). Child workflows expect local variables, which can be:
   - A **bare variable** (e.g. `reference_number`), or
   - A **nested variable** (e.g. `userform` can contain all the fields in the userform as a JSON object, and you can access them using dot notation).
2. **Initial Task Outputs**: The first assessment task (usually `fcau_1_0_apply`) must capture critical workflow state like `reference_number` and the user form object:
   ```json
   "output_mapping": {
     "reviewerform.reference_number": "fcau.reference_number",
     "userform": "fcau.userform",
     "reviewerform.application_review_outcome": "fcau.application_review_outcome"
   }
   ```
3. **Subworkflow Inputs**: All subsequent tasks (`type: TASK`) in the parent workflow **must** pass down these states via `input_mapping`. Leaving this empty will cause the child tasks to fail execution:
   ```json
   "input_mapping": {
     "fcau.reference_number": "reference_number",
     "fcau.userform": "userform"
   }
   ```
4. **Cross-Subworkflow / Inter-Task Variable Propagation**:
   - Subworkflows run in completely isolated execution contexts. They ONLY have access to variables mapped into them in the parent workflow's task node `input_mapping`.
   - If a subworkflow (e.g., `npqs-review-treatment-certs`) needs to access data produced in a previous subworkflow (e.g., `traderinput` from `npqs-upload-treatment-certs`), this data **must** be explicitly propagated:
     1. The producing subworkflow must return the variable in its outputs (e.g., `"traderinput"`).
     2. The parent workflow task node must map this output back to a parent global variable:
        ```json
        "output_mapping": {
          "traderinput": "npqs.treatment_traderinput"
        }
        ```
     3. The parent workflow task node invoking the subsequent subworkflow must map that parent variable to the child's input variable:
        ```json
        "input_mapping": {
          "npqs.treatment_traderinput": "traderinput"
        }
        ```
     Without this chain of input/output mappings, the child workflow interpreter will fail with an error like `input mapping error: required global variable 'traderinput' not found in workflow variables`.

---

## 3. Subworkflow Definitions (`workflow.json`)

The child subworkflow defines the execution path of a single transaction stage.

```json
{
  "id": "fcau-pay-app-fee-flow",
  "name": "Pay Application Fee",
  "version": 1,
  "nodes": [
    { "id": "start", "type": "START" },
    {
      "id": "pay_app_fee_task",
      "type": "TASK",
      "task_template_id": "fcau-pay-app-fee--payment",
      "input_mapping": {
        "reference_number": "reference_number"
      },
      "output_mapping": {
        "payment_status": "payment_status"
      }
    },
    { "id": "end", "type": "END" }
  ],
  "edges": [ ... ]
}
```

---

## 4. UI Rendering Configuration (`render.json`)

`render.json` instructs the trader-app (or officer-app) frontend how to lay out the workspace zones, what blueprints to load, and which interactions are legal.

### Schema Fields
- `id`: Unique identifier, conventionally `<subworkflow-id>:render` (e.g. `fcau-warehouse-scheduling-flow:render`).
- `type`: A user-facing category for the view. `APPLICATION` (trader/applicant submission view) and `REVIEW` (officer review split pane) are the common ones; the shipped configs also use `PAYMENT`, `SYSTEM`, `LAB_TEST`, `SAMPLE_COLLECTION`, `VISUAL_ASSESSMENT`, and `CERTIFICATE_ISSUANCE`.
- `title`: Human-readable name for the whole task view (e.g. `[Trade] Select HS Codes`).
- `read`: **Who may read this task at all** — see [Read authorization](#read-authorization) below.
- `sections`: Map of slots (e.g. `workspace`, `reference`, `instructions`).
  - `templateId`: Identifies the schema file to display (maps to `id` in the respective `*_jsonform.json`).
  - `projector`: `FORM` (interactive JSONForm), `MARKDOWN` (static instructions), or `PAYMENT` (checkout page).
  - `dataKey`: Variable name matching the task's output namespace (e.g. `traderinput`, `reviewerform`). Omit it to hand the projector the whole variable map.
  - `title`: Heading rendered above the section.
  - `visibleWhen`: Declarative rules deciding whether the section renders at all. All rules present must hold (they AND together); omitting the block renders the section always. There is no `OR` — express alternatives as separate slots.
    - `states`: List of task states the section shows in, matched case-insensitively (e.g. `["PENDING_USER"]`).
    - `requireDataKey`: Section only renders if this **top-level** key exists and is non-null in the task's data. Not a dotted path.
    - `requireClaim`: Section only renders if the caller holds this claim — see [Read authorization](#read-authorization).
  - `handles`: **CRITICAL FOR EDITABILITY**. Defines what actions/buttons can be clicked on the form zone. **If `handles` is missing or empty, the frontend renders the form fields as read-only (non-interactive).** A handle only reaches the frontend if its section rendered *and* its `command` is legal in the current state, so hiding a section also removes its buttons.
- `states`: Defines the operational lifecycle.
  - `PENDING_USER`: Active state where user can perform actions.
    - `actions`: List of allowed commands (e.g. `{ "command": "submit" }`).

### Read authorization

`GET /api/v1/tasks/{id}` is scoped to the caller. Before rendering, the backend resolves one **claim** per role in `configs/catalog.json`, named `role:<logicalName>` — so `role:trader` and `role:cha` today. A claim is true only when the caller both holds that role in their token **and** their company owns the task's consignment in that same slot; holding the role is not enough.

Two levers use those claims:

- **`read.roles`** (top level) lists the logical roles allowed to read the task at all. A caller eligible for none of them gets a `404` — deliberately indistinguishable from a task that does not exist. Omit the block (or leave the list empty) to admit any role that owns the consignment.
- **`visibleWhen.requireClaim`** (per section) gates one section on one claim, which is how a single task state shows different content to different roles.

> [!IMPORTANT]
> **`read.roles` is ordered, and the order is precedence.** One user can be eligible for several roles at once — a self-clearing operator holding both Trader and CHA at a company that is both the trader and the CHA on its own consignment. Since `visibleWhen` rules only ever AND, two true claims would render two contradictory sections side by side ("here is the form" next to "waiting for your CHA"). So when `read.roles` is declared, the caller **acts as the first role in the list they are eligible for**, and exactly one `role:*` claim is true.
>
> Put the role that *acts* on the task first: `["cha", "trader"]` for a step the CHA performs, `["trader", "cha"]` for one the trader performs. Reordering the list changes which screen a dual-role user gets.
>
> A config that declares **no** `read.roles` has expressed no precedence, so every eligible role's claim is reported. That is safe only because such a config has no per-role sections to disagree — **if you use `requireClaim` anywhere, declare `read.roles`.**

```json
{
  "id": "trade-hscode-selection-flow:render",
  "type": "APPLICATION",
  "read": { "roles": ["cha", "trader"] },
  "sections": {
    "status_message": {
      "templateId": "trade-hscode-selection--waiting-markdown",
      "projector": "MARKDOWN",
      "visibleWhen": { "states": ["PENDING_USER"], "requireClaim": "role:trader" }
    },
    "workspace": {
      "templateId": "trade-hscode-selection--form",
      "projector": "FORM",
      "dataKey": "traderinput",
      "visibleWhen": { "states": ["PENDING_USER"], "requireClaim": "role:cha" },
      "handles": [{ "command": "submit", "label": "Complete Selection", "element": "primary_action" }]
    }
  },
  "states": { "PENDING_USER": { "actions": [{ "command": "submit" }] } }
}
```

In the same `PENDING_USER` state the CHA gets the form and its submit button, while the trader gets only a waiting notice. A section with no `requireClaim` is visible to whichever role the caller is acting as — which is how both roles share one completed-summary section.

> [!WARNING]
> Claim names are matched **exactly and case-sensitively**, and a `requireClaim` naming a claim the backend does not produce fails the whole request with a `500` rather than silently hiding the section. Only use names of the form `role:<key>` where `<key>` is a role in `configs/catalog.json`.

> [!IMPORTANT]
> This is presentation-layer scoping and the read gate — it does not authorize *writes*. Who may run a command is a separate, deny-by-default rule in the subtask template's `authz` extension (see [`internal/tasks/extensions/stepauthz/README.md`](../internal/tasks/extensions/stepauthz/README.md)). A section hidden here still needs its command denied there.

### Interactive Form Template Example (`render.json`)
```json
{
  "id": "fcau-warehouse-scheduling-flow:render",
  "type": "APPLICATION",
  "sections": {
    "workspace": {
      "id": "workspace",
      "templateId": "fcau-warehouse-scheduling--form",
      "title": "Warehouse Inspection Scheduling",
      "projector": "FORM",
      "dataKey": "traderinput",
      "handles": [
        {
          "command": "submit",
          "label": "Schedule Inspection",
          "element": "primary_action"
        }
      ]
    }
  },
  "states": {
    "PENDING_USER": {
      "actions": [
        {
          "command": "submit"
        }
      ]
    }
  }
}
```

---

## 5. Forms Config and Schemas

### Task Types Configuration (`traderinput.json` / `officerinput.json`)
Declares if the task is completed by the applicant (`USER_INPUT`) or another agency (`EXTERNAL_REVIEW`), and sets up callback routes:
```json
{
  "id": "fcau-warehouse-inspection--officer-review",
  "task_type": "EXTERNAL_REVIEW",
  "output_namespace": "reviewerform",
  "plugin_properties": {
    "service_id": "fcau",
    "path": "/api/v1/inject",
    "task_code": "fcau_warehouse_inspection_v1"
  }
}
```

### JSONForm Schemas (`*_jsonform.json`)
Follows standard [JSONForms](https://jsonforms.io/) schemas with a `schema` and `uiSchema` block:
```json
{
  "id": "fcau-warehouse-scheduling--form",
  "title": "Schedule Warehouse Inspection",
  "schema": {
    "type": "object",
    "properties": {
      "inspection_date": { "type": "string", "format": "date", "title": "Preferred Date" }
    },
    "required": [ "inspection_date" ]
  },
  "uiSchema": {
    "type": "VerticalLayout",
    "elements": [
      { "type": "Control", "scope": "#/properties/inspection_date" }
    ]
  }
}
```

## 6. Development Workflow & Hot-Reloading

1. **Parent Workflow Hot-Reload**:
   - The parent workflow file `fcau_workflow.json` is read from disk on every new consignment initialization. Modifying this file does **not** require a server restart.
2. **Form schemas and markdown templates**:
   - `*_jsonform.json` and markdown templates are fetched through the artifact loader on **every render**, so edits show up on the next request with no restart (with `ARTIFACT_LOADER_TYPE=local`).
3. **Render configs**:
   - `render.json` is **snapshotted into `task_records_v2.render_config` when the task starts**, not read per request. Editing one therefore affects **newly created tasks only** — existing tasks keep the blob they were created with. To see a render-config change, start a **fresh consignment**.
4. **The manifest**:
   - `manifest.json` is read once at startup. Adding a new template file means adding its row there **and** restarting the server.
