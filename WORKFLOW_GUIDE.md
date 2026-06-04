# Single-Window Workflow & Task Template Configuration Guide

This document serves as an exhaustive reference for creating, modifying, and debugging workflows, task templates, JSONForms, and rendering configurations in the NSW (National Single Window) system. Keep this guide as a direct instruction manual for any AI model (Gemini, Claude, Antigravity, etc.) tasked with writing configuration files.

---

## 1. Directory Structure: Folder-as-Task Convention

Task definitions are grouped into self-contained folders representing micro-workflows under an agency-specific folder (e.g. `configs/<agency_code>/`). 

> [!NOTE]
> `fcau` (Food Control Administration Unit) is used as the reference example throughout this guide. For each agency process in our NSW system, we will write a similar set of configs (e.g. Coconut Development Authority (`cda`), National Plant Quarantine Service (`npqs`), etc.).

Each task folder is recognized by the `config_loader.go` registry scanner and must conform to the following file layout:

```
configs/<agency_code>/
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
- `type`: `APPLICATION` (trader/applicant submission view) or `REVIEW` (officer review split pane).
- `sections`: Map of slots (e.g. `workspace`, `reference`, `instructions`).
  - `templateId`: Identifies the schema file to display (maps to `id` in the respective `*_jsonform.json`).
  - `projector`: `FORM` (interactive JSONForm), `MARKDOWN` (static instructions), or `PAYMENT` (checkout page).
  - `dataKey`: Variable name matching the task's output namespace (e.g. `traderinput`, `reviewerform`).
  - `handles`: **CRITICAL FOR EDITABILITY**. Defines what actions/buttons can be clicked on the form zone. **If `handles` is missing or empty, the frontend renders the form fields as read-only (non-interactive).**
- `states`: Defines the operational lifecycle.
  - `PENDING_USER`: Active state where user can perform actions.
    - `actions`: List of allowed commands (e.g. `{ "command": "submit" }`).

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

### Critical Guidelines for Forms, Dropdowns, and Optional Fields (Common Gotchas)

1. **No Trailing Commas in JSON**:
   - JSON configurations (including form schemas, render settings, and subtask profiles) are loaded by standard JSON parsers that are strict. **Do not leave trailing commas in arrays (e.g. `required` lists) or object properties.** A single trailing comma will cause the entire config loader to fail on startup:
     ```json
     // INVALID JSON (fails to parse):
     "required": [
       "certificate_id",
     ]
     ```

2. **Safe Sanitization for Optional Fields & Dropdowns**:
   - If optional form fields (especially checkboxes or dropdowns/select fields) are not checked or selected by the user, their keys are omitted from the form data payload.
   - If they are omitted, output mappings in the Go/Temporal orchestration engine will fail with `output mapping error: required task variable '<key>' not found in task result` if the key is not present in the task result payload.
   - To prevent this:
     - The frontend runs a sanitization function (`sanitizeFormData`) right before form submission.
     - **For checkboxes/booleans**: If unchecked/missing, they default to `false` (or their schema-defined default).
     - **For dropdowns & input fields**: If empty/missing/unselected, they are automatically set to their schema-defined `default` value if present, or explicitly set to `null` if no default is specified.
     - Setting them to `null` ensures the key is present in the JSON payload, which resolves to `nil` in Go, satisfying the output mapping check safely without polluting the data with arbitrary strings.

---

## 6. Development Workflow & Hot-Reloading

1. **Parent Workflow Hot-Reload**:
   - The parent workflow file `fcau_workflow.json` is read from disk on every new consignment initialization. Modifying this file does **not** require a server restart.
2. **Subworkflows and Render configs**:
   - Subfolder configurations (e.g. `workflow.json`, `render.json`) are parsed and cached in-memory during application startup (`LoadConfigsInto`). **Modifying any render config, form schema, or child workflow definition requires restarting the Go server (`go run ./cmd/server`)**.
