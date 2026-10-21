package payloadschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

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

// fakeForms serves forms from a map, as the artifact registry would.
type fakeForms map[string]string

func (f fakeForms) form(_ context.Context, id string) ([]byte, error) {
	form, ok := f[id]
	if !ok {
		return nil, fmt.Errorf("form %q not found", id)
	}
	return []byte(form), nil
}

// naming returns the extension's properties naming form formID.
func naming(formID string) json.RawMessage {
	return json.RawMessage(`{"form": "` + formID + `"}`)
}

func TestExtension_Execute(t *testing.T) {
	ext := NewExtension(fakeForms{
		"review-form":     reviewForm,
		"schemaless-form": `{"id": "schemaless-form", "uiSchema": {}}`,
		"bad-schema-form": `{"id": "bad-schema-form", "schema": {"type": 5}}`,
	}.form)

	tests := []struct {
		name    string
		props   json.RawMessage
		draft   bool
		payload string
		// wantInvalid: the submission is rejected as not matching its form.
		// wantErr: it is rejected for another reason (misconfiguration).
		wantInvalid bool
		wantErr     bool
	}{
		{name: "valid submission", props: naming("review-form"), payload: `{"outcome": "approve"}`},
		{name: "system keys are not form data", props: naming("review-form"), payload: `{"outcome": "approve", "__command": "submit"}`},
		{name: "invalid submission", props: naming("review-form"), payload: `{"outcome": "banana"}`, wantInvalid: true},
		{name: "submission naming the draft command is still validated", props: naming("review-form"), payload: `{"__command": "save_as_draft"}`, wantInvalid: true},
		{name: "draft is skipped", props: naming("review-form"), draft: true, payload: `{"outcome": "banana"}`},
		{name: "no properties", payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "properties name no form", props: json.RawMessage(`{}`), payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "properties that aren't an object", props: json.RawMessage(`"review-form"`), payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "named form missing", props: naming("absent-form"), payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "form without a schema", props: naming("schemaless-form"), payload: `{"outcome": "approve"}`, wantErr: true},
		{name: "schema that doesn't compile", props: naming("bad-schema-form"), payload: `{"outcome": "approve"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.draft {
				ctx = WithDraft(ctx)
			}
			record := &store.TaskRecord{TaskID: "task-1", ActiveTaskTemplateID: "review-step"}

			err := ext.Execute(ctx, record, decode(t, tt.payload), tt.props)

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
	ext := NewExtension(fakeForms{"review-form": reviewForm}.form)
	err := ext.Execute(context.Background(), &store.TaskRecord{ActiveTaskTemplateID: "review-step"}, decode(t, `{}`), naming("review-form"))
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
