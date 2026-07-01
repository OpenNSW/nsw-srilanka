# Task-step authorization extension

A `PRE_RESUME` task extension that decides whether the caller may run a command on
a task at its current state. It is the enforcement point behind
`POST /api/v1/tasks/{id}`.

It is a **pure evaluator**: the API layer (a task-write middleware) resolves the
caller's identity and their ownership of the task's consignment and attaches an
`Input` to the request context. This extension only matches that `Input` against
the per-task policy and the global catalog — it never touches the consignment or
company services.

## Per-task config

Add an `authz` block to a subtask template's `extensions` array. `properties` is
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

The logical names resolve through the global catalog (`configs/task_authz.json`,
`TASK_AUTHZ_CONFIG_PATH`): `users` maps a name to a token role, `clients` maps a
name to an OAuth2 client id.

```json
{
  "users":   { "trader": "Trader", "cha": "CHA" },
  "clients": { "fcau": "FCAU_TO_NSW", "npqs": "NPQS_TO_NSW" }
}
```

## Decision

- **User** — allowed iff, for some allowed name, the caller holds that role **and**
  their company owns the consignment in that role (ownership resolved by the API
  layer, tied to the required role — holding the role is not enough).
- **Client (M2M)** — allowed iff an allowed name maps to the caller's client id.
- Otherwise `403`; no principal ⇒ `401`.
