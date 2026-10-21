# Payload schema validation extension

A `PRE_RESUME` task extension that checks a step submission against the JSON Schema of
the form the user filled in. The trader app already validates every form in the
browser; this extension applies the same schema on the server, so a call that skips the
browser is held to the same rules.

It runs wherever core runs `PRE_RESUME` extensions for a step that lists it: submissions
(`POST /api/v1/tasks/{id}/steps/{stepId}`), callbacks (`POST /api/v1/callbacks/{token}`)
and draft saves (`PUT /api/v1/tasks/{id}/steps/{stepId}/draft`).

## Enabling it on a step

List the extension on the step template, naming the form a submission must match in its
`form` property.

```json
{
  "id": "npqs-application-review--officer-review",
  "task_type": "USER_INPUT",
  "output_namespace": "reviewform",
  "plugin_properties": {
    "user_jsonforms_id": "npqs-apply-phyto-cert--reviewer-form"
  },
  "extensions": [
    {
      "id": "payload_schema",
      "phase": "PRE_RESUME",
      "properties": { "form": "npqs-apply-phyto-cert--reviewer-form" }
    }
  ]
}
```

The extension never reads `plugin_properties`; name the form the user fills in, which
on a `USER_INPUT` step is usually the one `user_jsonforms_id` shows.

Removing it from the step template turns validation off for that step. A step template
that lists it while the running backend doesn't register it fails every submission on
that step, so the artifacts must not list it before a release that has it is deployed.

## What it checks

The submission, without core's system keys (those prefixed `__`, such as `__command`), is
validated against the form's `schema`, read the way the trader app's Ajv instance reads
it:

- draft-07 when the schema declares no `$schema`;
- formats asserted, with ajv-formats' rules for `time` (timezone optional) and `email`
  (dot required in the domain); formats neither side defines, such as `file`, are ignored;
- standard JSON Schema semantics: `required` only checks that a key is present.

The schema is compiled on every call, as the other artifact templates are parsed on every
use, so an edited form applies at once.

## Outcome

- **Matches:** the step goes ahead.
- **Doesn't match:** `400` with the fixed message `submission does not match the form`.
  The failing fields (JSON pointer and the rule that failed, never the submitted value)
  are logged at warn level, not returned:
  the browser checks the same schema first, so this is either a form the browser and the
  server disagree on, or a call that skipped the browser.
- **Misconfigured** (no `form` in the extension's properties, an unknown form, a form
  without a `schema`, or a schema that doesn't compile): `500`. The extension fails closed; it never lets a
  submission through unchecked.

## Drafts

A draft may be incomplete, so draft saves are not checked against the form. The draft
route marks its request context (`WithDraft`), and the extension knows a draft by that
mark, never by the command: a submission that names the step's draft command is
validated in full.

## Not covered yet

- Read-only fields and fields the form doesn't declare
- Ajv-only keywords, external reviewer callbacks, and schemas built at render time
