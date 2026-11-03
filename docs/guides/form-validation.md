# Adding validation to a form

This guide is for whoever writes the forms in [`one-trade-artifacts`](https://github.com/OpenNSW/one-trade-artifacts). It shows how to write the common rules in a form's JSON Schema and UI schema, and where each one is checked.

A form is checked twice:

- **In the browser**, as the user fills it in. On a button that submits the form, the portal won't send one that breaks a rule.
- **On the server**, when the submission arrives, against the same form's schema. This holds a call that skips the browser to the same rules.

Both read the schema as JSON Schema draft-07. The server's exact rules are in the [payload schema extension's README](../../internal/tasks/extensions/payloadschema/README.md).

## Turn on the server check for a step

List the `payload_schema` extension on the step template, and name in `form` the form the step's render shows:

```json
"extensions": [
  {
    "id": "payload_schema",
    "phase": "PRE_RESUME",
    "properties": { "form": "npqs-apply-phyto-cert--reviewer-form" }
  }
]
```

Every step a person completes needs it, with a few listed exceptions, and the artifacts CI fails on one without it (see [Artifacts CI](#artifacts-ci)). A step no person completes, such as an `EXTERNAL_REVIEW` or `PAYMENT` step, must not list it.

## Rules

Each rule below is checked in the browser and on the server unless it says otherwise.

### A required field

List the field in its object's `required`:

```json
{
  "type": "object",
  "required": ["consignee_name"],
  "properties": {
    "consignee_name": { "type": "string", "title": "Consignee name" }
  }
}
```

For a field listed this way, the browser also treats a blank answer (an empty or all-space text, or an empty list) as missing. The server's `required` only checks that the field is there, so to refuse a blank answer on both sides, add a rule for it:

- **Text:** `"pattern": "\\S"` refuses an empty or all-space text; `"minLength": 1` refuses only an empty one.
- **A list:** `"minItems": 1`.

### A field required only for some answers

Put the condition in an `if`/`then` inside `allOf`, and show the field with a `SHOW` rule on the same condition:

```json
"allOf": [
  {
    "if": {
      "properties": { "decision": { "const": "reject" } },
      "required": ["decision"]
    },
    "then": { "required": ["rejection_reason"] }
  }
]
```

```json
{
  "type": "Control",
  "scope": "#/properties/rejection_reason",
  "rule": {
    "effect": "SHOW",
    "condition": {
      "scope": "#/properties/decision",
      "schema": { "const": "reject" }
    }
  }
}
```

Don't list a field that a rule can hide in the object's plain `required`: the browser still checks a hidden field, so the user can't submit, and no field shows why. The `required` inside the `if` makes the condition apply only once `decision` has an answer.

The browser's blank-answer check doesn't reach a field required inside an `allOf` or a `$ref`, including the `if`/`then` above, so add `pattern` or `minItems` as above if a blank answer must be refused.

### Text: length and format

```json
"vehicle_number": {
  "type": "string",
  "minLength": 3,
  "maxLength": 20,
  "pattern": "^[A-Za-z0-9/-]+$"
}
```

`pattern` is a regular expression that has to work in both JavaScript (browser) and Go (server). Keep to the common features: character classes, `\d`, `\s`, `\S`, `\p{L}`, groups, `|`, and the `?`, `*`, `+` and `{n,m}` repeats. Don't use lookaheads or lookbehinds (`(?=`, `(?!`, `(?<=`, `(?<!`) or back-references (`\1`): the server can't read the form, and every submission on the step fails. Anchor the expression with `^` and `$` to match the whole value; without them, it only has to match part of it.

In JSON, write each backslash twice: `"\\d"`.

### Numbers only

```json
"number_of_bags": { "type": "integer", "minimum": 1 },
"weight_of_bag": { "type": "number", "exclusiveMinimum": 0, "maximum": 50 },
"pack_size": { "type": "integer", "multipleOf": 5 }
```

- `"type": "number"` takes any number, `"type": "integer"` a whole one. A number field shows a number input, and its value is sent as a number, never as text.
- `minimum` and `maximum` include the limit; `exclusiveMinimum` and `exclusiveMaximum` don't.
- `multipleOf` takes only multiples of it. Give it a whole number: with a decimal one such as `0.01`, the browser refuses some valid values (`0.29`), as its arithmetic rounds them.
- With a lower limit of 0 or more, the input doesn't take `-`, `+`, `e` or `E`.

For digits kept as text, such as a phone number with a leading zero, use a string with a `pattern` (`"^[0-9]{10}$"`) instead.

### A choice from a list

Give each choice as a `const` with the `title` the user sees:

```json
"decision": {
  "type": "string",
  "title": "Decision",
  "oneOf": [
    { "const": "approve", "title": "Approve" },
    { "const": "reject", "title": "Reject" }
  ]
}
```

Only those values are accepted. The field shows as a dropdown; to show radio buttons instead, set `"options": { "format": "radio" }` on its control. An `enum` (`"enum": ["approve", "reject"]`) works too, but shows the values themselves.

### Dates and times

```json
"date_of_sailing": { "type": "string", "format": "date" },
"pickup_time": { "type": "string", "format": "time" },
"inspected_at": { "type": "string", "format": "date-time" }
```

- `date` is `2026-10-30`.
- `time` is `14:30:00`. It needs seconds, which the time picker adds.
- `date-time` is `2026-10-30T14:30:00+05:30`.

To keep one date no later than another field of the same object, give it a `formatMaximum` that points at that field:

```json
"date_of_containerization": {
  "type": "string",
  "format": "date",
  "title": "Date of containerization",
  "formatMaximum": { "$data": "1/date_of_sailing" }
}
```

`formatMinimum` (no earlier than), `formatExclusiveMaximum` (earlier than) and `formatExclusiveMinimum` (later than) work the same way. Write them only in this form, on a `date`, pointing at a field next to it with `"1/<field>"`.

### Email

```json
"contact_email": { "type": "string", "format": "email" }
```

The address needs a dot in its domain: `a@example.com` is accepted, `a@localhost` isn't.

### A value the server fills in

A value the workflow puts into the step, such as a reference number or a lab result, which the user should see but not change, is marked `readOnly` in the schema:

```json
"reference_number": { "type": "string", "title": "Reference number", "readOnly": true }
```

- The browser shows it greyed out, with a lock, and doesn't let the user change it.
- The server refuses a submission that changes it, leaves out a value the step holds, or fills in one the step doesn't (other than its `default`).
- The value has to be in the step's own data: the workflow's `input_mapping` writes it under the step's `output_namespace` when the step starts.
- Mark a whole object or list `readOnly` to keep all of it as it is; mark fields inside list rows to keep those values in each row.

Use `readOnly` only for values the server filled in. A value the browser makes, such as a row ID from `x-template`, isn't one, so it would be refused.

A UI schema `"options": { "readonly": true }` looks the same in the browser, greyed out with a lock, but the server doesn't read it: the value is treated like any other answer and can still be changed. Use it for display, never to keep a value from changing.

### Lists

```json
"containers": {
  "type": "array",
  "minItems": 1,
  "maxItems": 20,
  "uniqueItems": true,
  "items": {
    "type": "object",
    "required": ["container_no"],
    "properties": { "container_no": { "type": "string" } }
  }
}
```

- `minItems` and `maxItems` limit the number of rows; `uniqueItems` refuses two identical rows.
- Each row is checked against `items`, including its `required`.
- The user can add and remove rows unless the list's control turns that off: `"options": { "addable": false }`, `"options": { "removable": false }`. When rows hold `readOnly` values, the server follows the same switches: it refuses a new row carrying such values, and refuses a missing row unless the list is removable.

### Files

A file field is a string with `"format": "file"`, or a list of them, with its rules in `x-file`:

```json
"invoice": {
  "type": "string",
  "format": "file",
  "title": "Invoice (PDF)",
  "x-file": { "accept": ".pdf" }
},
"supporting_documents": {
  "type": "array",
  "items": { "type": "string", "format": "file" },
  "x-file": { "accept": ".pdf,.jpg,.jpeg,.png", "maxFiles": 5 }
}
```

- `accept` lists file name extensions (`.pdf`), or `*` for any file. Don't use types such as `application/pdf` or `image/*`: the server matches a file by its extension, so a type matches no file there. List only extensions the deployment accepts for upload (`storage.allowedUploadTypes`).
- `maxFiles` is the most files a list holds. A list needs `maxFiles` above 1; without it the file control keeps one file.
- `maxSize` (in bytes) limits a file's size in the browser, before upload; it defaults to 5 MiB. Every upload is also held to the deployment's `storage.maxUploadBytes`.
- To require at least one file in a list, add `"minItems": 1`.

### Other JSON Schema keywords

The rest of draft-07 works the same on both sides, so a form can use it as JSON Schema describes it: `const`, `enum`, `not`, `anyOf`, `oneOf`, `allOf`, `if`/`then`/`else`, `dependencies`, `minProperties`, `maxProperties`, `additionalProperties`, `patternProperties`, `propertyNames`, `contains`, `items` given as a list with `additionalItems`, and `$ref` to `definitions`.

`format` works the same on both sides for `date`, `time`, `date-time`, `email`, `uri`, `uri-reference`, `uri-template`, `hostname`, `ipv4`, `ipv6`, `uuid`, `duration`, `regex` and `json-pointer`.

Keep to those. In particular:

- **Other formats** (`url`, `byte`, `int32`, `iri`, …) are checked by only one of the two. `format: file` marks a file field, whose rules are in `x-file` (see [Files](#files)).
- **Keywords from later drafts** (`dependentRequired`, `unevaluatedProperties`, `prefixItems`, …) are ignored by both: a form doesn't declare its draft, and both read it as draft-07.
- **Next to a `$ref`**, write nothing else: put the extra rules in an `allOf` with the `$ref`. The server drops a rule written beside a `$ref`. In a form with any `readOnly` value, a `default` beside a `$ref`, or a keyword that holds a `readOnly` (such as `properties` or `allOf`), makes it refuse every submission on the step.
- **`readOnly`** is followed (the server checks the value) on properties and list items, and through `allOf`, `$ref` targets, `additionalProperties` and `patternProperties`. Inside `if`/`then`/`else`, `oneOf`, `anyOf`, `not`, `contains`, `propertyNames`, the schema form of `dependencies` (`"dependencies": {"a": {…}}`), `items` given as a list, or `additionalItems`, the server can't tell whether the value applies, so it refuses every submission on the step.

## Drafts and other buttons

The button's `element` in the render config decides what is checked:

| `element`                         | Browser        | Server                     |
| --------------------------------- | -------------- | -------------------------- |
| `primary_action`, `danger_action` | the whole form | the whole form             |
| `draft_action` (and the autosave) | nothing        | only the `readOnly` values |
| `secondary_action`                | nothing        | the whole form             |

A `secondary_action` is sent unchecked by the browser but checked in full on the server, so it suits a button that sends a complete form. On a button meant to send a form part-filled, the server refuses the submission.

## When a submission is refused

- **The browser** doesn't send the form, and shows the errors on the fields.
- **The server** answers `400`. Which fields failed, and why, is in the backend's warn log.
- A form the server can't read, such as one with an unsupported `pattern` or a `form` that doesn't exist, makes every submission on its step fail with a `500`.

## Artifacts CI

The CI in `one-trade-artifacts` checks two of these rules on every change:

- [`check_payload_schema.py`](https://github.com/OpenNSW/one-trade-artifacts/blob/main/.github/scripts/check_payload_schema.py): every step a person completes, other than those listed in its `EXEMPT`, lists `payload_schema`, with the form its render shows.
- [`check_file_fields.py`](https://github.com/OpenNSW/one-trade-artifacts/blob/main/scripts/check_file_fields.py): every file field has `x-file` with an `accept` of extensions, and a list has `maxFiles` above 1.

The repository's [`CLAUDE.md`](https://github.com/OpenNSW/one-trade-artifacts/blob/main/CLAUDE.md) has the full authoring rules.
