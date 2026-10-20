package payloadschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/OpenNSW/core/artifact/adapter/types"
	"github.com/OpenNSW/core/taskflow/store"
)

// reviewForm is a form template as the artifacts store it.
const reviewForm = `{
	"id": "review-form",
	"title": "Review",
	"schema": {
		"type": "object",
		"properties": {
			"outcome": {"type": "string", "oneOf": [{"const": "approve"}, {"const": "needs_more_info"}]},
			"reason": {"type": "string"}
		},
		"required": ["outcome"],
		"additionalProperties": false
	},
	"uiSchema": {"type": "VerticalLayout", "elements": []}
}`

// fakeArtifacts serves step templates and forms from maps, as the artifact
// registry would.
type fakeArtifacts struct {
	steps map[string]types.StepTemplate
	forms map[string]string
}

func (f fakeArtifacts) stepTemplate(_ context.Context, id string) (types.StepTemplate, error) {
	step, ok := f.steps[id]
	if !ok {
		return types.StepTemplate{}, fmt.Errorf("step template %q not found", id)
	}
	return step, nil
}

func (f fakeArtifacts) form(_ context.Context, id string) ([]byte, error) {
	form, ok := f.forms[id]
	if !ok {
		return nil, fmt.Errorf("form %q not found", id)
	}
	return []byte(form), nil
}

func stepNaming(formID string) types.StepTemplate {
	return types.StepTemplate{
		ID:               "review-step",
		PluginType:       "USER_INPUT",
		PluginProperties: json.RawMessage(`{"user_jsonforms_id": "` + formID + `"}`),
	}
}

func TestExtension_Execute(t *testing.T) {
	artifacts := fakeArtifacts{
		steps: map[string]types.StepTemplate{
			"review-step":     stepNaming("review-form"),
			"no-form-step":    {ID: "no-form-step", PluginType: "USER_INPUT"},
			"missing-form":    stepNaming("absent-form"),
			"schemaless-step": stepNaming("schemaless-form"),
			"bad-schema-step": stepNaming("bad-schema-form"),
		},
		forms: map[string]string{
			"review-form":     reviewForm,
			"schemaless-form": `{"id": "schemaless-form", "uiSchema": {}}`,
			"bad-schema-form": `{"id": "bad-schema-form", "schema": {"type": 5}}`,
		},
	}
	ext := NewExtension(artifacts.stepTemplate, artifacts.form)

	tests := []struct {
		name    string
		step    string
		draft   bool
		payload string
		// wantInvalid: the submission is rejected as not matching its form.
		// wantErr: it is rejected for another reason (misconfiguration).
		wantInvalid bool
		wantErr     bool
	}{
		{name: "valid submission", step: "review-step", payload: `{"outcome": "approve"}`},
		{name: "system keys are not form data", step: "review-step", payload: `{"outcome": "approve", "__command": "submit"}`},
		{name: "invalid submission", step: "review-step", payload: `{"outcome": "banana"}`, wantInvalid: true},
		{name: "submission naming the draft command is still validated", step: "review-step", payload: `{"__command": "save_as_draft"}`, wantInvalid: true},
		{name: "draft is skipped", step: "review-step", draft: true, payload: `{"outcome": "banana"}`},
		{name: "step names no form", step: "no-form-step", payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "unknown step template", step: "absent-step", payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "named form missing", step: "missing-form", payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "form without a schema", step: "schemaless-step", payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "schema that doesn't compile", step: "bad-schema-step", payload: `{"outcome": "approve"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.draft {
				ctx = WithDraft(ctx)
			}
			record := &store.TaskRecord{TaskID: "task-1", ActiveTaskTemplateID: tt.step}

			err := ext.Execute(ctx, record, decode(t, tt.payload), nil)

			switch {
			case tt.wantInvalid:
				var invalid *InvalidPayloadError
				if !errors.Is(err, ErrInvalidPayload) || !errors.As(err, &invalid) || len(invalid.Violations) == 0 {
					t.Fatalf("Execute() = %v; want an *InvalidPayloadError with violations", err)
				}
			case tt.wantErr:
				if err == nil || errors.Is(err, ErrInvalidPayload) {
					t.Fatalf("Execute() = %v; want a non-validation error", err)
				}
			default:
				if err != nil {
					t.Fatalf("Execute() = %v; want nil", err)
				}
			}
		})
	}
}

func TestExtension_Execute_MatchesErrInvalidPayloadWhenWrapped(t *testing.T) {
	ext := NewExtension(
		fakeArtifacts{steps: map[string]types.StepTemplate{"review-step": stepNaming("review-form")}}.stepTemplate,
		fakeArtifacts{forms: map[string]string{"review-form": reviewForm}}.form,
	)
	err := ext.Execute(context.Background(), &store.TaskRecord{ActiveTaskTemplateID: "review-step"}, decode(t, `{}`), nil)
	// The orchestrator wraps extension errors with %w before returning them.
	wrapped := fmt.Errorf("PRE_RESUME extension %q failed: %w", "payload_schema", err)

	var invalid *InvalidPayloadError
	if !errors.Is(wrapped, ErrInvalidPayload) || !errors.As(wrapped, &invalid) {
		t.Fatalf("wrapped error %v doesn't match ErrInvalidPayload", wrapped)
	}
}

func TestDraftMarker(t *testing.T) {
	if IsDraft(context.Background()) {
		t.Error("IsDraft on a plain context = true")
	}
	if !IsDraft(WithDraft(context.Background())) {
		t.Error("IsDraft after WithDraft = false")
	}
}
