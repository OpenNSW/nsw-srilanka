# Notifications

A workflow can send an **email or SMS** when a step is completed — for example,
"trader submits the FCAU application → confirmation email is sent".

You configure two things:

1. **Where to send** — gateway credentials in `notification.json`.
2. **When to send** — an `extensions` block on a workflow step.

---

## 1. Gateway credentials: `notification.json`

Copy `notification.example.json` to `notification.json` and fill in real values.
This file is gitignored, so secrets stay local.

```json
{
  "email": {
    "baseURL": "https://email.svc.local",
    "token": "your-token"
  },
  "sms": {
    "baseURL": "https://smsservice.lk",
    "userName": "your-username",
    "password": "your-password",
    "sidCode": "your-sender-id"
  }
}
```

Restart the server after changing this file.

---

## 2. Sending on a step: the `extensions` block

Add this to the workflow step that should send the message:

```json
"extensions": [
  {
    "id": "notification",
    "phase": "POST_RESUME",
    "properties": {
      "channel": "email",
      "to_path": "userform.contact_email",
      "subject": "Application received",
      "body": "Your application has been received and is now under review."
    }
  }
]
```

### Properties

| Field       | Required | What it is                                          |
| ----------- | -------- | --------------------------------------------------- |
| `channel`   | yes      | `"email"` or `"sms"`.                               |
| `to_path`   | yes      | Where to find the recipient (see below).            |
| `subject`   | email    | Email subject.                                      |
| `body`      | yes      | Message text. (email may use `html_body` instead.)  |
| `html_body` | no       | HTML body, email only.                              |
| `to`        | no       | Fixed fallback address if `to_path` finds nothing.  |
| `task_code` | no       | A label shown in the logs.                          |

### How `to_path` works

The recipient comes from data an **earlier step already saved**. Each step saves
its data under its `output_namespace`. So `to_path` is that namespace plus the
field name.

Example: the FCAU step has `"output_namespace": "userform"` and a `contact_email`
field, so the email address is at `userform.contact_email`.

If no recipient is found, nothing is sent — the workflow keeps running.

### `phase`

- `POST_RESUME` (recommended) — sends in the background; a failure is logged but
  never stops the workflow.
- `PRE_RESUME` — sends before the step finishes; a failure stops the step. Use
  only if the message must be sent for the step to complete.
