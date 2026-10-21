package payloadschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// FormOf returns the form template with the given id as the artifacts store it:
// a JSON object whose "schema" member is the form's JSON Schema.
type FormOf func(ctx context.Context, formID string) ([]byte, error)

// Extension is the PRE_RESUME task extension that validates a submission
// against the JSON Schema of the form the step names in the extension's
// properties. It runs on every step that lists it and fails closed: a step
// whose form can't be found or compiled rejects every submission rather than
// letting one through unchecked.
type Extension struct {
	formOf FormOf
}

// properties is the extension's configuration on a step template:
//
//	{"id": "payload_schema", "phase": "PRE_RESUME", "properties": {"form": "<form id>"}}
type properties struct {
	// Form is the id of the form whose schema a submission must match.
	Form string `json:"form"`
}

// NewExtension builds the extension over the lookup it reads forms through.
func NewExtension(formOf FormOf) *Extension {
	return &Extension{formOf: formOf}
}

// Execute runs in the PRE_RESUME phase, before the task resumes; a non-nil
// return aborts the step. A payload that doesn't match the form is reported as
// an *InvalidPayloadError, which matches ErrInvalidPayload; any other error is
// a misconfiguration or a failed lookup.
func (e *Extension) Execute(ctx context.Context, record *store.TaskRecord, payload map[string]any, props json.RawMessage) error {
	// A draft may be incomplete, so it is not held to the form's rules. The
	// read-only and undeclared-field checks, which do apply to drafts, are
	// added here by #795 and #796.
	if IsDraft(ctx) {
		return nil
	}

	formID, err := namedForm(props)
	if err != nil {
		return fmt.Errorf("payload schema: step template %q: %w", record.ActiveTaskTemplateID, err)
	}
	schema, err := e.schemaOf(ctx, formID)
	if err != nil {
		return err
	}
	compiled, err := compile(schema)
	if err != nil {
		return fmt.Errorf("payload schema: form %q: %w", formID, err)
	}

	if err := compiled.Validate(formData(payload)); err != nil {
		var invalid *jsonschema.ValidationError
		if errors.As(err, &invalid) {
			return &InvalidPayloadError{Violations: violations(invalid)}
		}
		return fmt.Errorf("payload schema: validate submission for step template %q: %w", record.ActiveTaskTemplateID, err)
	}
	return nil
}

// namedForm returns the id of the form the extension's properties name.
func namedForm(props json.RawMessage) (string, error) {
	var p properties
	if len(props) > 0 {
		if err := json.Unmarshal(props, &p); err != nil {
			return "", fmt.Errorf("extension properties: %w", err)
		}
	}
	if p.Form == "" {
		return "", errors.New(`extension properties name no "form"`)
	}
	return p.Form, nil
}

// schemaOf returns the JSON Schema of form formID.
func (e *Extension) schemaOf(ctx context.Context, formID string) ([]byte, error) {
	raw, err := e.formOf(ctx, formID)
	if err != nil {
		return nil, fmt.Errorf("payload schema: form %q: %w", formID, err)
	}
	var form struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(raw, &form); err != nil {
		return nil, fmt.Errorf("payload schema: form %q: %w", formID, err)
	}
	if len(form.Schema) == 0 || string(form.Schema) == "null" {
		return nil, fmt.Errorf("payload schema: form %q has no schema", formID)
	}
	return form.Schema, nil
}

// formData returns payload without core's system keys (those prefixed "__",
// such as "__command"). They are not form data, and core strips the same keys
// before it stores a submission.
func formData(payload map[string]any) map[string]any {
	data := make(map[string]any, len(payload))
	for k, v := range payload {
		if !strings.HasPrefix(k, "__") {
			data[k] = v
		}
	}
	return data
}
