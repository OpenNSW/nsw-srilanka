# E2E Replay Tests

Data-driven end-to-end tests for the NSW backend. Each business flow is a JSON file; adding coverage means writing JSON, not Go.

The generic engine (flow schema, variable store, step execution, polling) lives in [`internal/replay`](../../internal/replay). This package is the in-process wiring, the actor configs (`configs/`), the flow files (`flows/`), and the tests that run them (`runner_e2e_test.go`).

## How it works

The harness starts the full app in-process with `bootstrap.Build` (no test-only production seams) and serves it via `httptest.Server`. Two test-only concerns are handled entirely in the harness:

- **Real auth, no IdP.** The app runs the production authn middleware. The harness runs a local JWKS server (`signedauth_test.go`), points `cfg.Authn.JWKSURL` at it, and mints RS256 tokens matching `cfg.Authn` (issuer/audience/client_id). Both MEMBER and SERVICE tokens are minted from the actor configs; `withAuth`/`withScope` run unchanged.
- **Config path.** `TestMain` calls `os.Chdir(repoRoot)` so `bootstrap.Build`'s working-directory-relative `configs/` path resolves under `go test`. No production change.

Per-step identity is chosen by the flow's `actor`, which must match an actor `id` in one of the config files:
- `"trader"` → MEMBER user (authorization_code token), defined in `configs/members/trader.json`.
- `"<agencyId>"` (e.g. `"fcau"`) → SERVICE/M2M client (client_credentials token), defined in `configs/agencies/fcau.json`.

**External agency flows** are handled by a generic mock agency (`mockagency_test.go`) that receives injects from the app and, when a `callback` step fires, posts the configured callback payload back to complete the parked EXTERNAL_REVIEW task.

**Payment flows** are handled by a generic mock gateway (`mockgateway_test.go`) that resolves the payment reference and posts a gateway webhook to confirm the payment.

FCAU is the sample flow exercising both. Other agency or payment flows need only a new JSON flow file.

## Directory layout

```
test/e2e/replay/
├── configs/
│   ├── members/        # MEMBER actor configs (Trader, CHA, …) — identity for token minting
│   │   └── trader.json
│   ├── agencies/       # SERVICE/M2M agency configs — identity + inbound/outbound wire protocol
│   │   └── fcau.json
│   └── payments/       # Payment gateway configs — webhook path + optional identity
│       └── govpay.json
├── flows/              # Flow JSON files (one per business scenario)
│   ├── fcau_application_approve.json
│   └── trade_up_to_hscode.json
├── config_test.go           # All config types (MemberConfig, AgencyConfig, PaymentConfig) and JSON loaders
├── harness_test.go          # Full in-process app setup
├── signedauth_test.go       # RS256 token minting + local JWKS server
├── mockagency_test.go       # Config-driven mock agency (inject receiver + callback poster)
├── mockgateway_test.go      # Config-driven mock payment gateway (DB poll + webhook poster)
└── runner_e2e_test.go       # Test functions — one per flow file
```

## Running

```bash
make dev        # start the full dev stack (Postgres, Temporal, IDP, …)
make test-e2e   # stops the api container, then runs E2E=1 GOWORK=off go test ./test/e2e/...
```

`make test-e2e` sources `.env` automatically — ensure `GOWORK` is not set (or set to `off`) in `.env` so the go workspace does not interfere with module resolution.

Tests skip unless `E2E=1`. Run serially — workers share fixed Temporal task queues.

## Config schema

### `configs/members/<id>.json`

```json
{
  "id": "trader",
  "identity": {
    "clientID": "TRADER_PORTAL_APP",
    "roles": ["Trader"],
    "scopes": ["nsw:consignment:read", "nsw:consignment:write", "..."]
  }
}
```

### `configs/agencies/<id>.json`

```json
{
  "id": "fcau",
  "identity": {
    "clientID": "FCAU_TO_NSW",
    "roles": ["AgencyM2M"],
    "scopes": ["nsw:task:write", "nsw:consignment:read", "..."]
  },
  "inbound": {
    "endpoint": "POST /api/v1/inject",
    "taskIDField": "taskId"
  },
  "outbound": {
    "callbackPath": "/api/v1/tasks/{taskId}",
    "commandField": "command",
    "payloadField": "payload"
  }
}
```

| Section | Purpose |
|---|---|
| `identity` | IdP credentials — used to mint/validate the M2M token |
| `inbound` | The HTTP endpoint the mock agency exposes to receive injects from the NSW app |
| `outbound` | How the mock posts the callback back to the NSW app |

### `configs/payments/<id>.json`

```json
{
  "id": "govpay",
  "webhookPath": "/api/v1/payments/govpay/webhook"
}
```

`identity` is optional — current production gateways post unauthenticated webhooks. When a gateway is made protected, add an `identity` block (same shape as the agency `identity`) and a bearer token will be minted and included automatically.

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
| `actor` | Must match an `id` in `configs/members/` or `configs/agencies/`. |
| `method` | HTTP method. |
| `path` | URL path; `{{var}}` tokens interpolated. |
| `body` | JSON body; `{{var}}` in string values interpolated. |
| `expectStatus` | Expected status code (default 200). |
| `extract` | `varName → dot.notation.path` from the JSON response (e.g. `"consignment.id"`). |

#### Completing a USER_INPUT task

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
| `content` | Reviewer payload. `{{var}}` tokens interpolated. |
| `timeout` | Wait for the inject to arrive; default 30s. |

The mock agency waits until the app sends the inject for that task id, then posts the callback using the wire format defined in `configs/agencies/<id>.json`.

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
| `method` | Payment gateway id — must match an `id` in `configs/payments/`. |
| `status` | Gateway success status (default `"paid"`). |
| `timeout` | Wait for the payment record to appear; default 45s. |

---

## How to add a new flow

1. **Identify node display names** — the `wait` `node` selector is the root `title` in the task's `configs/<agency>/<step>/render.json`.

2. **Identify required payload fields** — for USER_INPUT tasks: every field in `output_mapping.json` without `?`, plus JSONForm required fields. For EXTERNAL_REVIEW (agency callback): every field in the reviewer task's `output_mapping.json` without `?`.

3. **Create the flow file** at `test/e2e/replay/flows/<name>.json`.

4. **Register a test** in `runner_e2e_test.go`:

   ```go
   func TestReplay_MyFlow(t *testing.T) {
       skipUnlessE2E(t)
       runFlow(t, newHarness(t), "my_flow.json")
   }
   ```

5. **For a new agency**: add `test/e2e/replay/configs/agencies/<id>.json`. No Go changes needed.

6. **For a new member actor** (e.g. CHA): add `test/e2e/replay/configs/members/<id>.json`. No Go changes needed.

7. **For a new payment gateway**: add `test/e2e/replay/configs/payments/<id>.json`. No Go changes needed.

## Troubleshooting

**`wait` hangs at the first task** — the `api` container is running and stealing Temporal tasks. Stop it: `docker compose stop api`.

**DB connection refused** — source `.env` (DB is published on `DB_PORT`, e.g. `55432`, not the in-container `5432`).

**`callback` times out** — the inject hasn't arrived yet. Increase `timeout`. Also verify the agency `id` in the flow matches a file in `configs/agencies/`.

**Flow hangs after a `wait` (task never completes)** — a required field is missing from the prior `submit` payload. Check `output_mapping.json`: every field without `?` must be included.

**`pay` times out** — the payment record hasn't been created. Increase `timeout` or check the payment method submit step succeeded.

**401/403 on agency callback** — the mock posts with a real agency bearer. Ensure the agency `clientID` (from `configs/agencies/<id>.json`) is in `AUTH_CLIENT_IDS` in `.env`.
