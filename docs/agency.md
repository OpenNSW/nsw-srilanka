# Agency mode

The same backend runs as TNSW or as a government agency (e.g. CDA), chosen by `mode`
in `config.yaml`. The modes are exclusive: a deployment is one or the other, never
both. Both share the task engine, Temporal, renderers, plugins, storage and
`/api/v1/tasks/{id}`. They differ at the edges:

|                      | TNSW (`mode: tnsw`)                                     | Agency (`mode: agency`)                    |
| -------------------- | ------------------------------------------------------- | ------------------------------------------ |
| Workflow starts from | `POST /api/v1/consignments` (trader)                    | `POST /api/v1/inject` (external system)    |
| Task ownership       | trader/CHA company owns the consignment                 | any catalog role, on any injected workflow |
| Task claims          | off                                                     | on (a hint, never required)                |
| Own routes           | consignments, admin, CHAs/companies, payments, webhooks | `inject`, `cases`, `cases/{id}`            |
| Catalog must map     | `trader`, `cha`                                         | no particular role (see Roles per task)    |
| Catalog              | `catalog` in `configs/config.yaml`                      | `catalog` in the agency's `config.yaml`    |
| Artifacts            | TNSW artifact root                                      | the agency's artifacts                     |

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

To check the branch still only touches those files (fetch first, so `main` is current):

```sh
git diff main --stat -- $(git ls-tree -r --name-only main)
```

Watch the migration number. `migrations/000018_create_agency_workflow.sql` takes the
next free slot; if main adds its own `000018`, renumber this one after the merge.

## Running as an agency

Set `mode: agency` in `config.yaml` (`backend.config` in the Helm values), then
give it the agency's own catalog (the token roles and TNSW's client) and artifacts, and
the agency API's identifier as its audience:

```yaml
catalog:
  roles:
    officer: Officer
  clients:
    tnsw: NSW_TO_AGENCY
artifactLoader:
  type: local
  local:
    root: configs/agency/artifacts
authn:
  audience: https://api.cda.nsw-agency.local
```

`configs/agency/cda/` is a complete example.

The injecting client's token needs the `nsw:workflow:inject` scope, and `nsw:task:write`
to reply on the agency's parked steps (see [Asking for more information](#asking-for-more-information)).

Officers need the token role of at least one role in the agency catalog (see
[Roles per task](#roles-per-task)) and the `nsw:consignment:read` and task scopes the
portal token already carries.

### UI

Run the same trader-app with these keys in `public/config.js` (or
`frontend.config` in the Helm values):

```js
APP_MODE: 'agency',
```

In agency mode `/consignments` lists cases (`GET /api/v1/cases`), and
`/consignments/:id` reuses `ConsignmentDetailScreen` with a `getCase` fetcher
(`GET /api/v1/cases/{id}`): the tasks of every workflow in the case, in the
consignment's `workflowNodes` shape. Opening a task uses the normal
`/api/v1/tasks/{id}`, authorized by the officer gate. The screens keep the
`/consignments` paths because task cards and the task screen link there directly.

The app has no roles of its own: it admits every signed-in user, and the backend
checks the token's catalog roles on each call.

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
   and starts nothing. Nor is it a new review round: where the artifacts are wired
   for replies, a resubmission comes back as a reply on the agency's own token
   (below), not as a repeat inject.
5. The workflow is started as its own case, so tasks spawned under it have
   `CaseID == taskId`. The agency's task authz gate (`agency.RoleGate`) reports
   ownership in every catalog role for any case ID that is an `agency_workflow` row, so
   `readauthz` and the write extension treat officers like any other owner. Officers
   use the normal `/api/v1/tasks/{id}` routes.

## Asking for more information

Where the artifacts are wired for it (a `replyToken` input and a `reply_command` on
the `EXTERNAL_REVIEW` steps), only the first round goes through inject. After that,
each side calls back on the other side's waiting step, by the token that step handed
over:

1. TNSW's `EXTERNAL_REVIEW` step injects with its token `tok_b` and parks.
2. The officer asks for more information. The review workflow's own `EXTERNAL_REVIEW`
   step replies on `tok_b` with `needs_more_info` and its own token `tok_x` as
   `payload.callbackToken`, then parks. The step maps what it sends under its
   `payload` input, and is also given the submission and the officer's review form,
   which the reply leaves out, so the officer still sees what they reviewed and
   decided while they wait.
3. TNSW keeps `tok_x` and sends the task back to the trader. On resubmit, the same
   `EXTERNAL_REVIEW` step replies on `tok_x` with the new data and its own token
   `tok_c`, then parks.
4. The agency step maps the new data to `submission` and `tok_c` to `callbackToken`,
   and loops back to the officer.
5. The final decision calls back on the latest token, as before, with no token of its
   own, and both sides finish.

A token names one waiting step, so a reply cannot complete a stale or different one:
core answers `409`. The step that sent the reply then fails without retrying and parks
for an admin, since no retry can succeed. Artifacts that are not wired for replies
keep injecting: their resubmission is still a repeat inject, which the agency takes
for a retry and does not deliver. How a step replies is in
[`docs/WORKFLOW_GUIDE.md`](WORKFLOW_GUIDE.md#task-types-configuration-traderinputjson--officerinputjson).

## Claiming tasks

An officer claims a task so the others can see who is working on it. A claim is a
hint, not a lock: submitting a step never checks it.

- `POST /api/v1/tasks/{id}/claim` claims the task for the caller. If another officer
  holds the claim, the caller takes it over, for example when the holder is away. A
  completed task can't be claimed (409).
- `POST /api/v1/tasks/{id}/release` releases the caller's own claim.
- `GET /api/v1/tasks/{id}` returns a `claim` object: the holder's `name` (their email
  for now), `claimed_at` and `mine`.

Any officer who can open the task may claim it; both routes need `nsw:task:write`.
Claims, takeovers and releases are recorded in the audit trail as task updates. Claims
are on in agency mode and off in TNSW, where the routes answer 404 and the task view
has no `claim`. The claim is stored on the task row (`task_records_v2.claimed_by` and
`claimed_at`, migration `000028`) and lasts across every step of the task. The full
contract is in `api/tasks.openapi.yaml`.

## Task configs

Each `taskCode` a caller may inject has a `task_config` artifact, registered in the
agency manifests under the task code as its id. An inject records the manifest version
current when it arrives, and the workflow runs on it:

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

## Roles per task

An agency has no per-case owner, so every injected workflow is owned in **every role of
the agency's catalog**. What each role may do is decided by the task's own rules, which
the agency's artifacts therefore declare for every task:

- the step's `authz` extension says which roles may run each command, e.g.
  `"PENDING_USER": { "submit": ["officer"] }`;
- the render config's `read.roles` says which roles may open the task, e.g.
  `["officer"]`.

A step without an `authz` block is refused on writes, so every step must declare one.
A render config without a `read` block is readable by every catalog role, so a task
that should be open to one role only must say so. For the syntax see [`WORKFLOW_GUIDE.md`](WORKFLOW_GUIDE.md)
(read authorization) and the
[authz extension](../internal/tasks/extensions/authz/README.md).

No role is required of the catalog; it maps the roles the agency's tasks name. To add one:

1. Create the role in the IdP (`idp/resources/government/<agency>.json`).
2. Map its logical name to that token role under `catalog.roles` in the agency's
   `config.yaml`, e.g. `reviewer: CDA Reviewer`.
3. Name it in the `authz` and `read.roles` of the tasks it may act on.

The case list and detail admit an officer holding the token role of any catalog role.
A role a task names that the catalog does not define matches nobody, so the task is
closed to everyone.

A step that waits for TNSW's reply, such as a review's `*--request-info` step parked
until the trader resubmits, is completed by TNSW's machine client rather than an
officer. The catalog maps that client under `clients` as `tnsw` (for example
`"tnsw": "NSW_TO_CDA"`, the client TNSW injects and replies as), and such a step's
`authz` names it: `"QUEUED_EXTERNALLY": { "resubmit": ["tnsw"] }`. Without the
mapping, TNSW's reply is refused with `403`.

A catalog that maps no role (for example one with only `clients`) still starts, but no
officer can open cases or act on tasks: the server logs a warning at start. And without
`authz` and `read.roles` on a task there is no role-based access control on it, since
every catalog role then owns it.

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
TNSW uses the same table: a consignment is a case with the same id, and `consignments`
holds only its trade data (`flow`, trader and CHA). TNSW starts and completes its flows
through `internal/caseflow` (`case_flows`), which applies the same state rule. The
agency still records its workflows in `agency_workflow`.

The inject API keeps `consignmentId`, and so does the workflow variable of that name,
because callers and artifacts speak trade. Internally it is the case id.

## Known simplifications

- **No "next task" button for officers.** On completion `TaskDetailScreen` looks up
  the next task via `/api/v1/consignments/{id}`, which 404s for a case, so the button
  never appears. The detail header also shows an empty trade-flow badge.
