# Replay E2E harness

This package runs **end-to-end tests by replaying an ordered list of API calls**
defined in a JSON *flow file*. It builds the real backend in-process and drives
it over HTTP against a real Postgres + Temporal (started with `make deps`). Each
business flow (trade export, FCAU, NPQS, …) is authored as **data** — a flow
file — so adding coverage means writing JSON, not Go.

The generic engine (flow schema, variable store, step execution, polling) lives
in [`internal/replay`](../../internal/replay). This package is the in-process
wiring (`harness_test.go`, `authsigned_test.go`), the flow files (`flows/`), and
the tests that run them (`runner_e2e_test.go`).

## How the harness builds the app

The app is assembled with the **unmodified** production `bootstrap.Build` (no
test-only seams) and served via an `httptest.Server` using `app.Server.Handler`.
Two test-only concerns are handled entirely in the harness:

- **Real authentication.** The app runs the production authn middleware. The
  harness starts a local JWKS server (`authsigned_test.go`), points
  `cfg.Authn.JWKSURL` at it, and mints RS256 tokens whose claims match
  `cfg.Authn` (issuer/audience/`client_id`). So the real `withAuth`/`withScope`
  run — **no IdP needed, and no user-token gap** (we mint both MEMBER and SERVICE
  tokens). The minted tokens carry a `roles` claim (`Trader`, `AgencyM2M`, …) for
  future role-based authz; nothing enforces roles yet.
- **Config path.** `Build` loads `configs/` relative to the working directory;
  `TestMain` does `os.Chdir(repoRoot)` so it resolves under `go test`. (No
  production change — the harness is purely additive test code.)

Per-step identity is chosen by the flow's `actor`, surfaced via the
`X-Auth-Actor` header that a client `RoundTripper` swaps for a real
`Authorization: Bearer <token>`:
- `trader` → a MEMBER user (authorization_code; the seeded `user_records` row).
- `fcau` → an agency SERVICE client (client_credentials).

Because the engine speaks only HTTP + JSON, the same flow files could later run
black-box against a deployed stack by swapping the token source.

## Running

```bash
make deps          # start db + migrations + temporal
make test-e2e      # stops the api container, sources .env (if present), runs E2E=1 go test
```

`make test-e2e` stops the `api` container deliberately: a running `api` polls the
same Temporal task queues and would steal the in-process workers' tasks. Restart
it with `docker compose start api` when you're done.

Requirements:
- `.env` sourced so `DB_*` / `TEMPORAL_*` match the running containers (the target
  sources it automatically; falls back to the current environment if absent).
- Tests **skip** unless `E2E=1`.
- Run serially (the default) — workers share fixed Temporal queues.

## Flow file schema

A flow is `{ "name": ..., "steps": [ ... ] }`. Each step is exactly one of:

### `request` — issue an HTTP call
```json
{ "name": "trader initializes consignment", "request": {
    "actor": "trader",                         // X-Auth-Actor identity
    "method": "POST",
    "path": "/api/v1/tasks/{{initTask}}",       // {{var}} interpolation
    "body": { "consignment_name": "E2E", "cha_company_id": "adam-pvt-ltd" },
    "expectStatus": 204,                        // default 200; fails on mismatch
    "extract": { "consignmentId": "id" }        // var <- response field (dot-notation, e.g. "consignment.id")
} }
```

### `wait` — poll the consignment until a workflow node matches
```json
{ "name": "wait for HS-code task", "wait": {
    "node": "Select HS Codes",     // substring match on the node display name
    "state": "IN_PROGRESS",        // required node state (omit = any)
    "into": "hsTask",              // store the matched node's task id into a var
    "timeout": "45s"
} }
```
Polls `GET /api/v1/consignments/{{consignmentId}}` (set `consignmentId` via an
earlier `extract`). On timeout it dumps the current nodes for debugging.

### `callback` — drive a mock external agency to respond
```json
{ "name": "FCAU approves", "callback": {
    "taskCode": "fcau_application_review_v1",   // matches the inject's taskCode
    "content": { "application_review_outcome": "approve", "reference_number": "REF-1" },
    "timeout": "30s"
} }
```
Requires a `replay.Agency` wired on the runner. Not used by the trade flow; see
*External-agency flows* below.

Strings in `path` and `body` may reference variables as `{{name}}`.

## Authoring a new flow

1. **Pick the workflow** under [`configs/`](../../configs) (e.g.
   `configs/trade/trade_workflow.json`) and follow its node graph.
2. **Find node display names** — the `wait` selector matches the value the API
   exposes as the node name, which is the **root `title`** of that task's
   `render.json` (see `taskDisplayName` in
   [internal/consignment/service.go](../../internal/consignment/service.go)).
   E.g. `configs/trade/2-hscode_selection/render.json` → `"[Trade] Select HS Codes"`.
3. **Find request payloads** — a user task's body is the fields its
   `userinput_jsonform.json` marks `required` (with `oneOf`/`const` for allowed
   values). E.g. the initialize step → `{ "consignment_name": "…", "cha_company_id": "adam-pvt-ltd" }`.
4. **Write the flow** in `flows/<name>.json`: a `create` request that extracts
   `consignmentId`, then alternating `wait` (until a task appears) + `request`
   (submit it) steps.
5. **Register it** with a test in [runner_e2e_test.go](runner_e2e_test.go):
   ```go
   func TestReplay_MyFlow(t *testing.T) {
       skipUnlessE2E(t)
       runFlow(t, newHarness(t), "my_flow.json")
   }
   ```

## External-agency flows (FCAU/NPQS/…)

Some tasks are `EXTERNAL_REVIEW`: the system POSTs an *inject* to an external
agency and parks the workflow until the agency calls back to `/api/v1/tasks`
with an OGA envelope (`{task_id, consignment_id, payload:{action, content}}`,
unwrapped by `unwrapOGACallback` in
[internal/tasks/http_handler.go](../../internal/tasks/http_handler.go)).

The harness models this with a **controllable mock agency**
([mockagency_test.go](mockagency_test.go)) wired into every harness:
- It serves `POST /api/v1/inject` and records each inject. The harness writes a
  temp services config pointing the `fcau` service at it, so EXTERNAL_REVIEW
  injects land here.
- The engine's `callback` step calls `Agency.Respond(taskCode, content, …)`: the
  mock waits for a matching inject, then POSTs the OGA envelope back to the app's
  `/api/v1/tasks` with a **real `fcau` bearer** (the same minted SERVICE token),
  so the production `withAuth`/`withScope` validate it. It uses the
  harness-provided callback base (the in-process app), not the inject's
  `serviceUrl` (which points at `cfg.Server.ServiceURL`).

See [flows/fcau_application_approve.json](flows/fcau_application_approve.json) for
a worked example: submit the application → the inject is received → an `approve`
callback advances the workflow to the pay-fee step.

> Authoring tip: a USER_INPUT task's submission must include **every field its
> node `output_mapping` references without a `?` suffix** (a `?` marks the field
> optional). E.g. the FCAU application maps `other_declarations` (no `?`), so the
> submission must include it even though the JSONForms schema lists it as
> optional — otherwise the task is woken but never completes.

## Troubleshooting

- **`wait` times out at the first task:** the `api` container is probably still
  running and stealing Temporal tasks — `docker compose stop api`.
- **DB connection refused:** source `.env` (DB is published on `DB_PORT`, e.g.
  `55432`, not the in-container `5432`).
- **401/403 on every request:** check the minted token claims match `cfg.Authn`
  (issuer/audience/`client_id`) and that the JWKS server started before `Build`.
