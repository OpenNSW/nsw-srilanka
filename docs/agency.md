# Agency mode

The same backend runs as TNSW or as a government agency (e.g. CDA), chosen by `mode`
in `config.yaml`. The modes are exclusive: a deployment is one or the other, never
both. Both share the task engine, Temporal, renderers, plugins, storage,
`/api/v1/tasks/{id}`, and the admin routes (engine status, resolve,
force-park, and the generic `GET /api/v1/admin/cases/{id}` —
`internal/workflowadmin`), gated on the admin scopes rather than ownership.
They differ at the edges:

|                      | TNSW (`mode: tnsw`)                                                         | Agency (`mode: agency`)                  |
| -------------------- | --------------------------------------------------------------------------- | ---------------------------------------- |
| Workflow starts from | `POST /api/v1/consignments` (trader)                                        | `POST /api/v1/internal/cases/{id}/flows` |
| Task ownership       | trader/CHA company's OU owns the consignment                                | the agency's own OU owns the case        |
| Task claims          | off (its task templates leave out `claimable`)                              | on: its task templates set `claimable`   |
| Own routes           | consignments, `admin/consignments/{id}`, CHAs/companies, payments, webhooks | `internal/cases/{id}/flows`, `cases`     |
| Catalog must map     | `trader`, `cha`                                                             | no particular role (see Roles per task)  |
| Catalog              | `catalog` in `configs/config.yaml`                                          | `catalog` in the agency's `config.yaml`  |
| Artifacts            | TNSW artifact root                                                          | the agency's artifacts                   |

A missing or unknown `mode` stops the server at startup.

## Keeping the branch mergeable

Agency code lives outside the files main owns, so `main` can be merged in often
without conflicts. Main-branch files touched:

- `cmd/server/config`: the `mode` key (`mode.go`, and the `Mode` field in
  `config.go`).
- `internal/bootstrap/app.go`: `Build` branches on the mode. TNSW builds the
  consignment service and router, the trader/CHA task gate and its integration
  handlers, and mounts their routes with `mountTNSW` (`internal/bootstrap/tnsw.go`).
  An agency builds none of those, and instead gets `mountAgency` from
  `internal/bootstrap/agency.go`. Both build `internal/caseflow` and the task gate. Most of
  the diff is indentation (`git diff -w`). Routes both modes serve stay in `Build`.
- `portals/apps/trader-app/src/App.tsx`, one import plus the `/consignments` and
  `/consignments/:consignmentId` routes choosing the agency screen when `isAgencyMode`.
  Everything else is in `src/features/case/`.

To check the branch still only touches those files (fetch first, so `main` is current):

```sh
git diff main --stat -- $(git ls-tree -r --name-only main)
```

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
flows:
  clients:
    enabled: true
    ouHandle: cda
    caseColumns: { name: /traderCompany/name }
    workflows:
      - { id: cda_officer_verification, clients: [tnsw], owner: officer }
artifactLoader:
  type: local
  local:
    root: configs/agency/artifacts
authn:
  audience: https://api.cda.nsw-agency.local
```

`configs/agency/cda/` is a complete example.

`flows.clients` lists the workflows TNSW may start, one per officer task (see
[Client flows](#client-flows)). TNSW's client token needs the `workflow:inject` scope to
start them, and `task:write` to reply on the agency's parked steps (see [Asking for more information](#asking-for-more-information)).

Officers need the token role of at least one role in the agency catalog (see
[Roles per task](#roles-per-task)) and the `consignment:read` and task scopes the
portal token already carries.

### UI

Run the same trader-app with these keys in `public/config.js` (or
`frontend.config` in the Helm values):

```js
APP_MODE: 'agency',
```

In agency mode `/consignments` lists cases (`GET /api/v1/cases`), and
`/consignments/:id` reuses `ConsignmentDetailScreen` with a `getCase` fetcher
(`GET /api/v1/cases/{id}`): the tasks of every workflow in the case whose
`read.roles` name a catalog role the officer holds, in the consignment's
`workflowNodes` shape. Opening a task uses the normal
`/api/v1/tasks/{id}`, authorized by the task gate. The screens keep the
`/consignments` paths because task cards and the task screen link there directly.

The app has no roles of its own. It shows each screen and button only to a user whose token
carries the scope its API calls need, for example `consignment:read` for the cases and
`task:write` for working on a task; anyone else sees **Access Restricted** in place of a
screen. An agency portal's `IDP_SCOPES` leaves out the scopes for creating consignments,
their flows and the admin views, so it never shows them. To give one role a different view
than another (a supervisor, say), grant the two IdP roles different scopes on the agency's
API. The backend still checks the token's catalog roles on each call.

The user menu lists the roles in the token's `roles` claim (`IDP_ROLE_CLAIM_NAME` names
another claim), each by its own name, or by the title and description `IDP_ROLE_LABELS`
gives it. The list is for display only; the screens and buttons follow the scopes.

## Flow

1. TNSW calls `POST /api/v1/internal/cases/{caseId}/flows` with
   `{workflowId, flowId, input, callbackToken, attributes}`. `caseId` is the consignment's
   id, and `attributes` the case's (see [Cases](#cases)), when it has any. A body over
   `server.maxRequestBytes` is a `413 request body too large`; one with any other field,
   or anything after the JSON value, is a `400 invalid request body`.
2. The caller must be a client the catalog maps, and `flows.clients` must list
   `workflowId` for it. Anything else is a `404 flow not found`, as for a flow that
   does not exist, and the reason is logged.
3. `caseflow.StartFlow` records the flow in `case_flows` (creating the case if it is
   new, and reopening it if it had finished) and, in the same transaction, the agency's
   own OU (`flows.clients.ouHandle`) as an owner of the case in the flow's `owner` role
   (`case_parties`), so a recorded flow always has its owner. The same transaction
   merges `attributes` into the case's and fills the columns `flows.clients.caseColumns`
   maps from them. It then starts `workflowId`
   with `flowId` as the workflow instance ID, on the manifest version current when the
   flow was first recorded. The workflow is seeded with `taskId` (the flow ID), `taskCode` (the
   workflow), `consignmentId` (the case), `notification` (the input) and, when the
   caller sent one, `callbackToken`. A workflow sends its decision back with it on the
   caller's `POST /api/v1/callbacks/{callbackToken}`: the token names the caller's
   step, so a late or repeated decision cannot complete a later one.
4. Retries are safe. A flow still `STARTING` (a failed start) is started again, with the
   owner already recorded, and a started one is returned without touching the engine. A
   repeat `flowId` with a different case or workflow is a 409 and starts nothing. Nor is it a new review round: where the artifacts are wired for
   replies, a resubmission comes back as a reply on the agency's own token (below).
5. Every task the workflow spawns is filed under the case ID. The task authz gate
   (`authzgate`, the same as TNSW's) reports ownership in every role the officer's OU
   (their token's `ouHandle`) holds on the case: for an officer, the agency's own OU in
   the flow's `owner` role. `readauthz` and the write extension then treat officers like
   any other owner. Officers use the normal `/api/v1/tasks/{id}` routes.

## Asking for more information

Where the artifacts are wired for it (a `replyToken` input and a `reply_command` on
the `EXTERNAL_REVIEW` steps), only the first round starts a flow. After that,
each side calls back on the other side's waiting step, by the token that step handed
over:

1. TNSW's `EXTERNAL_REVIEW` step starts the agency's flow with its token `tok_b` and parks.
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
start the flow again: their resubmission is a repeat start, which the agency takes for a
retry and does not deliver. How a step replies is in
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

Any officer who can open the task may claim it; both routes need `task:write`.
Claims, takeovers and releases are recorded in the audit trail as task updates. Claims
are on in agency mode and off in TNSW, where the routes answer 404 and the task view
has no `claim`. The claim is stored on the task row (`taskflow_tasks.claimed_by` and
`claimed_at`, migration `000028`) and lasts across every step of the task. The full
contract is in `api/tasks.openapi.yaml`.

## Client flows

`flows.clients` in the agency's `config.yaml` lists the workflows another system may
start, who may start each (catalog `clients`), and the catalog role the agency's OU owns
the case in once it starts (`owner`). `ouHandle` is the agency's own OU, the `ouHandle`
its officers' tokens carry. The server refuses to start if a listed workflow is missing
from the current manifest, an `owner` or client is not in the catalog, or the list has
no `ouHandle`. An agency also refuses to start with the list off or empty: TNSW could
start no flow on it, so no officer task would ever be created.

```yaml
flows:
  clients:
    enabled: true
    ouHandle: cda
    workflows:
      - { id: cda_officer_verification, clients: [tnsw], owner: officer }
      - { id: cda_payment_review_receipt, clients: [tnsw], owner: accountant }
```

`caseColumns` fills `cases` columns from the attributes a flow is started with. Each key
is a column and each value the JSON Pointer (RFC 6901) of a string in the attributes.
`name` is the only column it can fill: `caseColumns: { name: /traderCompany/name }` makes
the trader company's name the case's name, which the case list shows over the case id. A
value that is missing, blank, or longer than the 255 characters `cases.name` holds leaves
the name as it is. The server refuses to start if a column is not `name` or a value is not
a JSON Pointer. The attributes are stored whether or not anything maps them.

`owner` decides who sees the case and its tasks: a role every officer holds for an
open case, or a narrower one (an accounts division's `accountant`) for a closed one.
The case detail titles each task by its render config. TNSW has no client flows: it
starts its own flows in code, at consignment creation.

## Roles per task

A client flow's case is owned in **the roles the agency's own OU holds on it**
(`case_parties`), like a TNSW consignment by its trader and CHA. What each role may do
is decided by the task's own rules, which the agency's artifacts therefore declare for
every task:

- the step's `authz` extension says which roles may run each command, e.g.
  `"PENDING_USER": { "submit": ["officer"] }`;
- the task template's `read.roles` says which roles may open the task, e.g.
  `["officer"]`.

A step without an `authz` block is refused on writes, so every step must declare one.
A task template without a `read` block is readable by every role that owns the case, so a task
that should be open to one role only must say so. The case detail, though, lists a task
only when its `read.roles` name a role the officer holds, so a task without them can be
opened but never appears in a case: give every task `read.roles`. The roles are copied
to the task when it starts, so changing a template's `read.roles` only affects tasks
started after the change. For the syntax see [`WORKFLOW_GUIDE.md`](WORKFLOW_GUIDE.md)
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
`"tnsw": "NSW_TO_CDA"`, the client TNSW starts flows and replies as), and such a step's
`authz` names it: `"QUEUED_EXTERNALLY": { "resubmit": ["tnsw"] }`. Without the
mapping, TNSW's reply is refused with `403`.

A catalog that maps no role (for example one with only `clients`) still starts, but no
officer can open cases or act on tasks: the server logs a warning at start. Ownership
only admits an officer to a case; the task decides the rest. A step without `authz`
refuses every write, and a task template without `read.roles` can be opened by every
role the officer's OU holds on the case.

## Cases

`cases` is the generic grouping of the workflows that work on one real-world matter.
One case has many flows (`case_flows.case_id`). It carries the columns of
`consignments` that are not trade-specific (`id`, `name`, `state`, `created_by`,
`created_at`, `updated_at`); `flow` belongs in its own table joined on case id.

`case_parties` records who holds which logical role on a case, one party (an OU, by its
handle, or a user) per role. The role is a catalog role name, so a new role needs no new
column. In TNSW the OU of a consignment's trader company holds `trader` and the OU of the
CHA company it selected holds `cha`. A user owns a task in every role their OU (their
token's `ouHandle`) holds on the task's case. An agency's own OU owns the cases its client
flows start, in each flow's `owner` role, and the case list and detail show an officer
only those (see [Roles per task](#roles-per-task)). Code: `internal/caseparty`.

`state` is `IN_PROGRESS` on creation and becomes `FINISHED` once **every** workflow of
the case has completed. A new flow started on a finished case reopens it
(`IN_PROGRESS`). `name` is empty until a caller or UI supplies one: in an agency,
`flows.clients.caseColumns` fills it from the attributes TNSW sends.

`attributes` is a JSON object of values about the case that the systems its flows run on
may see. TNSW sets it when it creates a consignment, to the trader company's name
(`{"traderCompany": {"name": "…"}}`), and `EXTERNAL_REVIEW` sends it with every flow it
starts on an agency. Only the top-level keys `consignment.SharedCaseAttributes` lists are
sent (`traderCompany` today), so a key written to TNSW's `cases.attributes` stays in TNSW
until it is listed there. The trader company's id and data, and the consignment's own
name, are never written. An agency merges the attributes a new flow
starts with into its own case, by top-level key. A new key needs no migration and no
agency change. Cases created before this have none, and keep showing their id.

Both modes start and complete flows through `internal/caseflow` (`case_flows`): its
completion callback marks the flow `COMPLETED`, and its case finished if it was the
last. The case row is locked while deciding, so two flows completing at once cannot
both leave it `IN_PROGRESS`. In TNSW a consignment is a case with the same id, and
`consignments` holds only its trade data (`flow`); its trader and CHA are the case's
parties.

The workflow variable `consignmentId` keeps its name because the artifacts speak trade.
It holds the case id.

## Known simplifications

- **Empty trade-flow badge.** The detail header shows an empty trade-flow badge for a
  case.
