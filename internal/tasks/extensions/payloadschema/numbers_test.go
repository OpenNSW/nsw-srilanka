package payloadschema

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// levyForm is a form whose numbers set no maximum of their own: an amount, a
// list of quantities, and a read-only total the server stored.
const levyForm = `{
	"id": "levy-form",
	"schema": {
		"type": "object",
		"properties": {
			"levy_amount": {"type": "number", "exclusiveMinimum": 0},
			"lines": {"type": "array", "items": {"type": "object", "properties": {
				"quantity": {"type": "number"}
			}}},
			"total": {"type": "number", "readOnly": true}
		}
	},
	"uiSchema": {"type": "VerticalLayout", "elements": []}
}`

func TestExtension_Execute_NumberLimit(t *testing.T) {
	ext := NewExtension(fakeForms{"levy-form": levyForm, "review-form": reviewForm}.form, namespace("reviewform"))
	// The stored total is past the limit: a read-only value is the server's,
	// so it is not held to it.
	stored := `{"reviewform": {"total": 1e20}}`

	tests := []struct {
		name    string
		form    string
		draft   bool
		payload string
		want    []Violation
	}{
		{name: "the largest amount", form: "levy-form", payload: `{"levy_amount": 9999999999999.99, "total": 1e20}`},
		{name: "an amount past the limit", form: "levy-form", payload: `{"levy_amount": 1e36, "total": 1e20}`,
			want: []Violation{{"/levy_amount", msgNumberTooLarge}}},
		{name: "a negative number past the limit", form: "levy-form", payload: `{"levy_amount": 1, "lines": [{"quantity": 1}, {"quantity": -1e16}], "total": 1e20}`,
			want: []Violation{{"/lines/1/quantity", msgNumberTooLarge}}},
		{name: "a number the schema doesn't name", form: "review-form", payload: `{"outcome": "approve", "extra": 1e16}`,
			want: []Violation{{"", "'additionalProperties' failed"}, {"/extra", msgNumberTooLarge}}},
		{name: "a draft is not held to it", form: "levy-form", draft: true, payload: `{"levy_amount": 1e36, "total": 1e20}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.draft {
				ctx = WithDraft(ctx)
			}
			err := ext.Execute(ctx, decodeRecord(t, stored), decode(t, tt.payload), naming(tt.form))

			if tt.want == nil {
				if err != nil {
					t.Fatalf("Execute() = %v; want nil", err)
				}
				return
			}
			var invalid *InvalidPayloadError
			if !errors.As(err, &invalid) {
				t.Fatalf("Execute() = %v; want an *InvalidPayloadError", err)
			}
			if fmt.Sprint(invalid.Violations) != fmt.Sprint(tt.want) {
				t.Errorf("violations = %v, want %v", invalid.Violations, tt.want)
			}
		})
	}
}
