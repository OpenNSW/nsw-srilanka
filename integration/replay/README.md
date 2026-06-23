# E2E Replay Tests

Data-driven end-to-end tests for the NSW backend. Each business flow is a JSON file; adding coverage means writing JSON, not Go.

The generic engine (flow schema, variable store, step execution, polling) lives in [`internal/replay`](../../internal/replay). This package is the in-process wiring, the flow files (`flows/`), and the tests that run them (`runner_e2e_test.go`).

## How it works

The harness starts the full app in-process with `bootstrap.Build` (no test-only production seams) and serves it via `httptest.Server`. Two test-only concerns are handled entirely in the harness:

- **Real auth, no IdP.** The app runs the production authn middleware. The harness runs a local JWKS server (`authsigned_test.go`), points `cfg.Authn.JWKSURL` at it, and mints RS256 tokens matching `cfg.Authn` (issuer/audience/client_id). Both MEMBER and SERVICE tokens are minted; `withAuth`/`withScope` run unchanged.
- **Config path.** `TestMain` calls `os.Chdir(repoRoot)` so `bootstrap.Build`'s working-directory-relative `configs/` path resolves under `go test`. No production change.

Per-step identity is chosen by the flow's `actor`:
- `"trader"` → the seeded MEMBER user (authorization_code token).
- `"<agencyId>"` (e.g. `"fcau"`) → an agency SERVICE client (client_credentials token).

**External agency flows** are handled by a generic mock agency (`mockagency_test.go`) that receives injects from the app and, when a `callback` step fires, posts `{command, payload}` back to complete the parked EXTERNAL_REVIEW task.

**Payment flows** are handled by a generic mock gateway (`mockgateway_test.go`) that resolves the payment reference and posts a gateway webhook to confirm the payment.

FCAU is the sample flow exercising both. Other agency or payment flows need only a new JSON flow file.

## Running

```bash
make deps          # start Postgres + Temporal
docker stop nsw-srilanka-api-1  # stop the api container (its workers compete with in-process ones)
source .env        # DB_*, TEMPORAL_* must match running containers
make test-e2e      # E2E=1 go test ./integration/replay/...
```

Tests skip unless `E2E=1`. Run serially — workers share fixed Temporal task queues.

## Flow file schema

```json
{ "name": "my_flow", "steps": [ ... ] }
```

Each step has a `name` and exactly one of: `request`, `wait`, `callback`, `pay`.

### Variables

`{{varName}}` in paths and body string values is interpolated from the variable store. Variables are populated by `wait` (via `into`) and `request` (via `extract`).

---

### `request` — issue an HTTP call

```json
{
  "name": "trader creates consignment",
  "request": {
    "actor": "trader",
    "method": "POST",
    "path": "/api/v1/consignments",
    "body": { "key": "value" },
    "expectStatus": 201,
    "extract": { "consignmentId": "id" }
  }
}
```

| Field | Notes |
|---|---|
| `actor` | Flow identity (see above). |
| `method` | HTTP method. |
| `path` | URL path; `{{var}}` tokens interpolated. |
| `body` | JSON body; `{{var}}` in string values interpolated. |
| `expectStatus` | Expected status code (default 200). |
| `extract` | `varName → dot.notation.path` from the JSON response (e.g. `"consignment.id"`). |

#### Completing a USER_INPUT task

All task completions use a single unified form:

```json
{
  "name": "trader initializes consignment",
  "request": {
    "actor": "trader",
    "method": "POST",
    "path": "/api/v1/tasks/{{initTask}}",
    "body": {
      "command": "submit",
      "payload": { "consignment_name": "My Consignment", "cha_company_id": "adam-pvt-ltd" }
    },
    "expectStatus": 204
  }
}
```

The `command` is `"submit"` for user-facing tasks. The `payload` must include every field that the task's `output_mapping.json` references **without** a `?` suffix (required fields), plus every required field in its JSONForm schema.

---

### `wait` — poll until a workflow node reaches a state

```json
{
  "name": "wait for Initialize Consignment task",
  "wait": {
    "node": "Initialize Consignment",
    "state": "IN_PROGRESS",
    "into": "initTask",
    "timeout": "45s"
  }
}
```

| Field | Notes |
|---|---|
| `node` | Substring match on the node display name (the render config's root `title`). |
| `state` | Required node state (e.g. `"IN_PROGRESS"`, `"COMPLETED"`). Omit to match any state. |
| `into` | Variable to store the matched node's task id (used by later steps). |
| `timeout` | Poll timeout; default 45s. On timeout, current nodes are dumped for debugging. |

Polls `GET /api/v1/consignments/{{consignmentId}}` (set by an earlier `extract`).

---

### `callback` — drive the mock agency to complete an EXTERNAL_REVIEW task

```json
{
  "name": "agency approves the application",
  "callback": {
    "taskVar": "fcauApp",
    "command": "approve",
    "content": {
      "application_review_outcome": "approve",
      "reference_number": "REF-001"
    },
    "timeout": "60s"
  }
}
```

| Field | Notes |
|---|---|
| `taskVar` | Name of the flow variable holding the task id (set by a prior `wait` with `into`). |
| `command` | Outcome command sent to NSW (e.g. `"approve"`, `"reject"`). |
| `content` | Reviewer payload; sent as `payload` in `{command, payload}`. `{{var}}` tokens interpolated. |
| `timeout` | Wait for the inject to arrive; default 30s. |

The mock agency waits until the app sends the inject for that task id, then posts `{"command": "...", "payload": {...}}` to `POST /api/v1/tasks/{taskId}` with a real agency bearer token.

The `content` fields must satisfy the reviewer task's `output_mapping.json` (every field without `?`). The `command` must be a valid outcome for the workflow gateway.

---

### `pay` — drive the mock gateway to confirm a payment task

```json
{
  "name": "payment gateway confirms the fee",
  "pay": {
    "taskVar": "payTask",
    "status": "paid",
    "timeout": "60s"
  }
}
```

| Field | Notes |
|---|---|
| `taskVar` | Name of the flow variable holding the pay task id. |
| `status` | Gateway success status (default `"paid"`). |
| `timeout` | Wait for the payment record to appear; default 45s. |

The mock gateway polls the payment store for the reference created against the task, then posts a gateway webhook to confirm it, advancing the workflow past the pay step.

---

## How to add a new flow

1. **Identify node display names** — the `wait` `node` selector is the root `title` in the task's `configs/<agency>/<step>/render.json`.

2. **Identify required payload fields** — for USER_INPUT tasks: every field in `output_mapping.json` without `?`, plus JSONForm required fields. For EXTERNAL_REVIEW (agency callback): every field in the reviewer task's `output_mapping.json` without `?`.

3. **Create the flow file** at `integration/replay/flows/<name>.json`.

4. **Register a test** in `runner_e2e_test.go`:

   ```go
   func TestReplay_MyFlow(t *testing.T) {
       skipUnlessE2E(t)
       runFlow(t, newHarness(t), "my_flow.json")
   }
   ```

5. **For a new agency**: add its service id to `agencyIDs` in `writeServicesConfig` (`harness_test.go`). No other Go changes needed — the mock agency and engine are generic.

## Worked example: FCAU application approve

`flows/fcau_application_approve.json` covers the full happy path:

1. Create consignment → wait for init task → submit `{command:"submit", payload:{consignment_name, cha_company_id}}`
2. Wait for HS-code task → submit `{command:"submit", payload:{hs_codes:["fcau-health-certificate-reg"]}}`
3. Wait for FCAU application task → submit `{command:"submit", payload:{all 15 application fields}}`
4. `callback` (`taskVar:"fcauApp"`, `command:"approve"`, reviewer content)
5. Wait for pay-fee task → submit `{command:"submit", payload:{selected_method:"..."}}`
6. `pay` (`taskVar:"payTask"`)
7. Wait for pay-fee `COMPLETED`

## Troubleshooting

**`wait` hangs at the first task** — the `api` container is running and stealing Temporal tasks. Stop it: `docker compose stop api`.

**DB connection refused** — source `.env` (DB is published on `DB_PORT`, e.g. `55432`, not the in-container `5432`).

**`callback` times out** — the inject hasn't arrived yet (prior `wait` caught `IN_PROGRESS` before the inject fired). Increase `timeout`. Also verify the agency service id is listed in `writeServicesConfig`.

**Flow hangs after a `wait` (task never completes)** — a required field is missing from the prior `submit` payload. Check `output_mapping.json`: every field without `?` must be included, even if the JSONForm schema marks it optional.

**`pay` times out** — the payment record hasn't been created. Increase `timeout` or check the payment method submit step succeeded.

**401/403 on agency callback** — the mock posts with a real agency bearer. Ensure the agency service id (e.g. `"fcau"`) is in `AUTH_CLIENT_IDS` in `.env`.
