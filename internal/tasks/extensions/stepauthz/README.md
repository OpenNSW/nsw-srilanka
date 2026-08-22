# `stepauthz` — task-step authorization extension

A `PRE_RESUME` task extension that decides whether the caller may run a command on
a task at its current state. It is the enforcement point behind
`POST /api/v1/tasks/{id}`.

It is a **pure evaluator**: Layer 1 ([`internal/tasks/authzgate`](../../authzgate))
resolves the caller's identity and a lazy resolver for their ownership of the
task's consignment, and attaches a `taskauthz.Input` to the request context. This
extension only matches that `Input` against the per-task policy and the global
catalog — it never touches the consignment or company services.

The principal types, the catalog slice, and the eligibility rule all live in
[`internal/tasks/taskauthz`](../../taskauthz), shared with the read path. This
package holds only the write policy.

## Per-task config

Add an `authz` block to a subtask template's `extensions` array. The block id
stays `"authz"` — it is the `ExtensionConfig.id` the artifacts declare, and is
independent of this package's name. `properties` is
`state → command → [logical principal names]`; deny-by-default (a state/command
with no rule is rejected):

```json
"extensions": [
  {
    "id": "authz",
    "phase": "PRE_RESUME",
    "properties": {
      "PENDING_USER":      { "submit": ["cha"] },
      "QUEUED_EXTERNALLY": { "approve": ["fcau"], "reject": ["fcau"] }
    }
  }
]
```

## Catalog

The logical names resolve through the global catalog (`configs/catalog.json`,
`CATALOG_CONFIG_PATH`): `roles` maps a name to an IdP token role, `clients` maps a
name to an OAuth2 client id.

```json
{
  "roles":   { "trader": "Trader", "cha": "CHA" },
  "clients": { "fcau": "FCAU_TO_NSW", "npqs": "NPQS_TO_NSW" }
}
```

The composition root loads the file once (`internal/catalog`) and injects the
`roles`/`clients` slice of it here, so this package reads no configuration file.

## Decision

- **User** — allowed iff, for some allowed name, the caller holds that role **and**
  their company owns the consignment in that role (ownership resolved by the API
  layer, tied to the required role — holding the role is not enough).
- **Client (M2M)** — allowed iff an allowed name maps to the caller's client id.
- Otherwise `403`; no principal ⇒ `401`.

## Reads are a separate evaluator

This extension only guards `POST` (running a command). `GET /api/v1/tasks/{id}` is
guarded by [`internal/tasks/readauthz`](../../readauthz) — a separate package
because core has no read hook: extensions only fire on `CompleteTaskStep`.

The two share Layer 1 (`authzgate` attaches the same `Input` to both routes) and
the same role-tied ownership rule, but differ in where the policy lives and what
denial means:

|            | write (this package)                     | read (`readauthz`)                          |
|------------|------------------------------------------|---------------------------------------------|
| policy in  | subtask template `extensions[].authz`    | `render.json` `read.roles`                   |
| keyed on   | `[state, command]`                       | the task as a whole                          |
| default    | deny (no rule ⇒ forbidden)               | any role owning the consignment              |
| denial     | `403`                                    | `404`, so task ids cannot be probed          |

`readauthz` additionally resolves a `role:<name>` claim per catalog role, which
render configs use to decide which sections a given role sees.

Both ask `taskauthz` the same question — which roles is this caller eligible for
on this task — and differ only in what they do with several answers. This
extension allows the write if **any** allowed role matches, because it is making
one yes/no decision. A read selects *content*, so two matches would render two
contradictory sections at once; `read.roles` is therefore an ordered precedence
list and the reader acts as exactly one role. See the read authorization section
of [`docs/WORKFLOW_GUIDE.md`](../../../../docs/WORKFLOW_GUIDE.md).
