# Task-step authorization extension

A `PRE_RESUME` task extension that decides whether the caller may run a command on
a task at its current state. It is the enforcement point behind both task-write
routes, `POST /api/v1/tasks/{id}/steps/{stepId}` and `POST /api/v1/callbacks/{token}`:
both complete a step through core's `CompleteTaskStep`, which runs this extension.

Every step a caller completes or drafts must declare it. The task manager is built
with core's `WithRequiredPreResumeExtensions("authz")`, so a step whose template has no
`authz` extension is refused to everyone (`ErrStepNotGuarded`, answered with `403`)
rather than open to anyone. Nothing turns that off.

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

The logical names resolve through the global catalog, the `catalog` section of
`config.yaml`: `roles` maps a name to an IdP token role, `clients` maps a name to an
OAuth2 client id.

```yaml
catalog:
  roles:
    trader: Trader
    cha: CHA
  clients:
    fcau: FCAU_TO_NSW
    npqs: NPQS_TO_NSW
```

The composition root decodes it with the rest of the config (`internal/catalog`) and
injects the `roles`/`clients` slice of it here, so this package reads no configuration
file.

The name `system` is reserved (`taskauthz.SystemPrincipal`): `Register` refuses a
catalog that defines it as a role or a client.

## System

Some steps are completed by this service itself, on the authority of an event it was
told about rather than a request: the payment service settling a payment, and the
customs and SLPA webhook handlers resuming the step a callback answers. They reach
the task manager through `internal/tasks/systemcaller`, which attaches the **system**
principal (`taskauthz.WithSystem`). A rule names them as `system`, under the command
each one sends:

```json
"properties": {
  "QUEUED_EXTERNALLY": { "submit": ["system"] }
}
```

No request can carry the system principal: Layer 1 maps a request only to a user or
a client, and the catalog can't define `system`, so neither can match it.

## Decision

- **User** — allowed iff, for some allowed name, the caller holds that role **and**
  their company owns the consignment in that role (ownership resolved by the API
  layer, tied to the required role — holding the role is not enough).
- **Client (M2M)** — allowed iff an allowed name maps to the caller's client id.
- **System** — allowed iff `system` is an allowed name.
- Otherwise `403`; no principal ⇒ `401`.

## Who may act

`MayAct` asks the same rules whether the caller may run **any** command on the
task in its current state, without running one. `MayActOnStep` asks it of every
`PRE_RESUME` `authz` extension on the active step's template, all of which must
allow it, as core runs them all. A step with none is one no reader may act on,
since the task manager refuses it (see above).

`GET /api/v1/tasks/{id}` uses `MayActOnStep` to decide whether the reader's file
references can be submitted back: a reader who may act gets `read` and `submit`,
anyone else `read` only. A draft counts as acting: core authorizes it as the
command its step template names for drafts, so a reader who may save a draft
gets `submit` too.
