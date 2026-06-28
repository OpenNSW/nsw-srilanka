# Notifications

Send an **email or SMS** when a workflow step completes — e.g. "trader submits
the FCAU application → confirmation email is sent".

Two things to configure: **where to send** (gateway credentials) and **when to
send** (an `extensions` block on a step).

## 1. Gateway credentials — `notification.json`

Copy `notification.example.json` to `notification.json` (gitignored) and fill in
real values. Restart the server after changing it.

```json
{
  "email": { "baseURL": "https://email.svc.local", "token": "your-token" },
  "sms":   { "baseURL": "https://smsservice.lk", "userName": "...", "password": "...", "sidCode": "..." }
}
```

The email `token` may be a literal, or a secret reference: `"env:EMAIL_TOKEN"`
(environment variable) or `"file:/run/secrets/email_token"` (file contents).

## 2. Sending on a step — the `extensions` block

```json
"extensions": [
  {
    "id": "notification",
    "phase": "POST_RESUME",
    "properties": {
      "channel": "email",
      "subject": "Application received",
      "body": "Your application is now under review."
    }
  }
]
```

| Property      | Required | What it is                                       |
| ------------- | -------- | ------------------------------------------------ |
| `channel`     | yes      | `"email"` or `"sms"`.                            |
| `body`        | yes\*    | Message text. SMS uses only this.                |
| `subject`     | email    | Email subject.                                   |
| `html_body`   | no       | HTML body, email only (auto-escaped).            |
| `template_id` | no       | Personalised template instead of inline text.   |
| `task_code`   | no       | Label shown in logs.                             |

\* `body`/`subject`/`html_body` may come from `template_id` instead of inline.

**`phase`:** use `POST_RESUME` — sends in the background, a failure is logged but
never blocks the workflow. (`PRE_RESUME` sends before the step finishes and a
failure stops the step; use only if the message is required for completion.)

## Recipient

The recipient is **not** set on the extension. The completing step's form must
include a field named `notifyRecipient`; its submitted value is used as the
address. The field is optional: if it's missing or empty, nothing is sent and
the workflow continues (logged at `Info` as a skip, not an error).

## Personalised messages — `template_id`

Instead of static `subject`/`body`, point `template_id` at a template document
to weave in data the trader entered. The document is JSON with up to three
[Go template](https://pkg.go.dev/text/template) fields:

```json
{
  "subject":   "Application received — {{.userform.exporter_name}}",
  "body":      "Dear {{.userform.exporter_name}}, your application is under review.",
  "html_body": "<p>Dear {{.userform.exporter_name}}, your application is under review.</p>"
}
```

- **Data** = accumulated workflow state, namespaced by each step's
  `output_namespace`. With `"output_namespace": "userform"`,
  `{{.userform.exporter_name}}` is the applicant's name.
- **Per field, template wins over the inline fallback.** No `template_id` → the
  inline fields are used directly.
- **A missing variable errors** (`{{.userform.typo}}` fails rather than rendering
  blank) — a broken template is caught, not sent half-empty.
- **`html_body` is auto-escaped**; `subject` and `body` are plain text.

**Register the document** in `configs/manifest.json` as a `generic_template`
whose `id` matches `template_id`, then restart the server:

```json
{
  "id": "fcau-application-received--notification",
  "kind": "generic_template",
  "loader": "local",
  "path": "fcau/1-application/notification_email.json"
}
```
