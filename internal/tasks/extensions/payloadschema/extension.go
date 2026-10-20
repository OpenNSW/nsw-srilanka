package payloadschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/OpenNSW/core/artifact/adapter/types"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// StepTemplateOf returns the step template with the given id.
type StepTemplateOf func(ctx context.Context, stepTemplateID string) (types.StepTemplate, error)

// FormOf returns the form template with the given id as the artifacts store it:
// a JSON object whose "schema" member is the form's JSON Schema.
type FormOf func(ctx context.Context, formID string) ([]byte, error)

// Extension is the PRE_RESUME task extension that validates a submission
// against the JSON Schema of the form its step template names in
// plugin_properties.user_jsonforms_id. It runs on every step that lists it and
// fails closed: a step whose form can't be found or compiled rejects every
// submission rather than letting one through unchecked.
type Extension struct {
	stepTemplateOf StepTemplateOf
	formOf         FormOf
}

// NewExtension builds the extension over the lookups it reads step templates
// and forms through.
func NewExtension(stepTemplateOf StepTemplateOf, formOf FormOf) *Extension {
	return &Extension{stepTemplateOf: stepTemplateOf, formOf: formOf}
}

// Execute runs in the PRE_RESUME phase, before the task resumes; a non-nil
// return aborts the step. A payload that doesn't match the form is reported as
// an *InvalidPayloadError, which matches ErrInvalidPayload; any other error is
// a misconfiguration or a failed lookup. The extension takes no properties.
func (e *Extension) Execute(ctx context.Context, record *store.TaskRecord, payload map[string]any, _ json.RawMessage) error {
	// A draft may be incomplete, so it is not held to the form's rules. The
	// read-only and undeclared-field checks, which do apply to drafts, are
	// added here by #795 and #796.
	if IsDraft(ctx) {
		return nil
	}

	schema, err := e.schemaOf(ctx, record.ActiveTaskTemplateID)
	if err != nil {
		return err
	}
	compiled, err := compile(schema)
	if err != nil {
		return fmt.Errorf("payload schema: form of step template %q: %w", record.ActiveTaskTemplateID, err)
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

// schemaOf returns the JSON Schema of the form step template stepTemplateID
// names in plugin_properties.user_jsonforms_id.
func (e *Extension) schemaOf(ctx context.Context, stepTemplateID string) ([]byte, error) {
	step, err := e.stepTemplateOf(ctx, stepTemplateID)
	if err != nil {
		return nil, fmt.Errorf("payload schema: step template %q: %w", stepTemplateID, err)
	}
	var props struct {
		UserJSONFormsID string `json:"user_jsonforms_id"`
	}
	if len(step.PluginProperties) > 0 {
		if err := json.Unmarshal(step.PluginProperties, &props); err != nil {
			return nil, fmt.Errorf("payload schema: plugin properties of step template %q: %w", stepTemplateID, err)
		}
	}
	if props.UserJSONFormsID == "" {
		return nil, fmt.Errorf("payload schema: step template %q names no form in plugin_properties.user_jsonforms_id", stepTemplateID)
	}

	raw, err := e.formOf(ctx, props.UserJSONFormsID)
	if err != nil {
		return nil, fmt.Errorf("payload schema: form %q of step template %q: %w", props.UserJSONFormsID, stepTemplateID, err)
	}
	var form struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(raw, &form); err != nil {
		return nil, fmt.Errorf("payload schema: form %q: %w", props.UserJSONFormsID, err)
	}
	if len(form.Schema) == 0 || string(form.Schema) == "null" {
		return nil, fmt.Errorf("payload schema: form %q has no schema", props.UserJSONFormsID)
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
