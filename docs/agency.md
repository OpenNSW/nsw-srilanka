# Agency mode

The same backend runs as TNSW or as a government agency (e.g. CDA), chosen by `mode`
in `config.yaml`. The modes are exclusive: a deployment is one or the other, never
both. Both share the task engine, Temporal, renderers, plugins, storage and
`/api/v1/tasks/{id}`. They differ at the edges:

|                      | TNSW (`mode: tnsw`)                                     | Agency (`mode: agency`)                  |
| -------------------- | ------------------------------------------------------- | ---------------------------------------- |
| Workflow starts from | `POST /api/v1/consignments` (trader)                    | `POST /api/v1/inject` (external system)  |
| Task ownership       | trader/CHA company owns the consignment                 | `officer` role, on any injected workflow |
| Own routes           | consignments, admin, CHAs/companies, payments, webhooks | `inject`, `cases`, `cases/{id}`          |
| Catalog must map     | `trader`, `cha`                                         | `officer`                                |
| Catalog              | `configs/catalog.json`                                  | `configs/agency/catalog.json`            |
| Artifacts            | TNSW artifact root                                      | the agency's artifacts                   |

A missing or unknown `mode` stops the server at startup.

## Keeping the branch mergeable

Agency code lives outside the files main owns, so `main` can be merged in often
without conflicts. Main-branch files touched:

- `cmd/server/config`: the `mode` key (`mode.go`, and the `Mode` field in
  `config.go`).
- `internal/bootstrap/app.go`: `Build` branches on the mode. TNSW builds the
  consignment service and router, the trader/CHA task gate and its integration
  handlers, and mounts their routes with `mountTNSW` (`internal/bootstrap/tnsw.go`).
  An agency builds none of those, and instead gets `agencyCompletion`,
  `newAgencyTaskGate` and `mountAgency` from `internal/bootstrap/agency.go`. Most of
  the diff is indentation (`git diff -w`). Routes both modes serve stay in `Build`.
- `portals/apps/trader-app/src/App.tsx`, one import plus the `/consignments` and
  `/consignments/:consignmentId` routes choosing the agency screen when `isAgencyMode`.
  Everything else is in `src/features/case/`.
- `portals/apps/trader-app/src/components/Layout/TopBar.tsx`, one import plus hiding
  the role switcher when `isAgencyMode`.

To check the branch still only touches those files (fetch first, so `main` is current):

```sh
git diff main --stat -- $(git ls-tree -r --name-only main)
```

Watch the migration number. `migrations/000018_create_agency_workflow.sql` takes the
next free slot; if main adds its own `000018`, renumber this one after the merge.

## Running as an agency

Set `mode: agency` in `config.yaml` (`backend.config` in the Helm values), then
point it at the agency's own catalog and artifacts, and accept the injecting client:

```sh
cp configs/agency/catalog.example.json configs/agency/catalog.json   # set the officer token role
```

```yaml
server:
  catalogConfigPath: configs/agency/catalog.json
artifactLoader:
  type: local
  local:
    root: configs/agency/artifacts
authn:
  clientIDs: [TRADER_PORTAL_APP, ..., NSW_TO_CDA]
```

`configs/agency/cda/` is a complete example.

The injecting client's token needs the `nsw:workflow:inject` scope.

Officers need the `officer` token role (mapped in the agency catalog) and the
`nsw:consignment:read` and task scopes the portal token already carries.

### UI

Run the same trader-app with these keys in `public/config.js` (or
`frontend.config` in the Helm values):

```js
APP_MODE: 'agency',
IDP_TRADER_ROLE_NAME: '<officer IDP role>',   // see below
```

In agency mode `/consignments` lists cases (`GET /api/v1/cases`), and
`/consignments/:id` reuses `ConsignmentDetailScreen` with a `getCase` fetcher
(`GET /api/v1/cases/{id}`): the tasks of every workflow in the case, in the
consignment's `workflowNodes` shape. Opening a task uses the normal
`/api/v1/tasks/{id}`, authorized by the officer gate. The screens keep the
`/consignments` paths because task cards and the task screen link there directly.

The app only admits users whose IDP group maps to one of its roles, and it has no
officer role. Mapping the officer role through `IDP_TRADER_ROLE_NAME` admits officers
as the app's "Trader" role. That is internal only: agency mode hides the role
switcher, the backend checks the real officer token role, and agency mode shows none
of the trader-only screens. Add a proper `officer` UI role when the agency UI grows.

## Flow

1. `POST /api/v1/inject` with `{taskId, taskCode, consignmentId, callbackToken, data}`.
2. The `taskCode` is resolved to its `task_config` (see below). An unknown code is a 400
   `unknown task code "<code>"` and records nothing.
3. `agency.Service.Inject` upserts the `cases` row keyed by `consignmentId` and records
   one `agency_workflow` row per `taskId` (status `STARTING`) in the same transaction.
   It then starts the config's `workflow` with `taskId` as the instance ID and marks
   it `STARTED`.
   The payload is seeded as the `notification` variable, and the `callbackToken`, when
   the caller sent one, as the `callbackToken` variable. Both are recorded on the row,
   so a retried start runs with the values from the first inject. A workflow sends its
   decision back with it on the caller's `POST /api/v1/callbacks/{callbackToken}`: the
   token names the caller's step, so a late or repeated decision cannot complete a
   later one.
4. Retries are safe. A row still `STARTING` (a failed start) is started again, and a
   `STARTED` row is returned without touching the engine. That matters: once a
   workflow completes, Temporal would accept the same ID as a new run. A repeat
   `taskId` with a different `consignmentId` or `taskCode` is not a retry: it is a 409
   and starts nothing.
5. Tasks spawned under the workflow have `RootWorkflowID == taskId`. The agency's task
   authz gate (`agency.OfficerGate`) reports `officer` ownership for any root that is
   an `agency_workflow` row, so `readauthz` and the write extension treat officers
   like any other owner. Officers use the normal `/api/v1/tasks/{id}` routes.

## Task configs

Each `taskCode` a caller may inject has a `task_config` artifact, registered in the
agency manifest under the task code as its id:

```json
{
  "schemaVersion": 2,
  "taskCode": "cda_officer_verification_v1",
  "workflow": "cda_officer_verification_v1",
  "meta": { "title": "CDA Application Verification", "description": "...", "category": "CDA" }
}
```

This is `schemaVersion` 2, as defined by `schemas/taskconfig.v2.schema.json` in
`one-trade-artifacts`; version 1 configs are rejected. It follows the sibling
nsw-agency service's `task_config` (same kind name, lookup by manifest id, parsed and
validated on every load, no startup validation), but that service reads version 1,
which defines the task inline. Version 2 has `workflow` in place of its `forms`,
`behavior` and `permissions`: here the workflow's
own artifacts (render config, review workflow) decide screens and outcomes. `meta`
names and describes the task in the case detail view (falling back to the render
title). `taskCode` in the file is informational; the manifest id is the key.
Code: `internal/agency/taskconfig` and its `taskconfigart` artifact adapter.

TNSW has no task configs: it picks its workflow server-side at consignment creation.

## Cases

`cases` is the generic grouping of the workflows that work on one real-world matter.
One case has many workflows (`agency_workflow.case_id`). It carries the columns of
`consignments` that are not trade-specific (`id`, `name`, `state`, `created_at`,
`updated_at`); `flow` and the trader/CHA detail belong in their own table joined on
case id.

`state` is `IN_PROGRESS` on creation and becomes `FINISHED` once **every** workflow of
the case has completed. A new workflow injected into a finished case reopens it
(`IN_PROGRESS`). `name` is empty until a caller or UI supplies one.

Completions reach the agency through `agency.CompletionHandler`, the parent runner's
completion callback in agency mode: the workflow is marked `COMPLETED`, and its case
finished if it was the last. A completion for anything that is not an injected
workflow is an error, since nothing else runs parent workflows in an agency. The case
row is locked while deciding, so two workflows completing at once cannot both leave it
`IN_PROGRESS`.
The intended next step, as a separate PR to main, is to move the trader/CHA columns
off `consignments` into such a table and retire `consignments` in favour of `cases`.
Until then TNSW writes only `consignments` and the agency only `cases`; nothing mirrors
rows between them.

The inject API keeps `consignmentId`, and so does the workflow variable of that name,
because callers and artifacts speak trade. Internally it is the case id.

## Known simplifications

- **No "next task" button for officers.** On completion `TaskDetailScreen` looks up
  the next task via `/api/v1/consignments/{id}`, which 404s for a case, so the button
  never appears. The detail header also shows an empty trade-flow badge.

- **Repeat injects are not delivered to the running workflow.** They return the
  existing row; nothing is signalled.
