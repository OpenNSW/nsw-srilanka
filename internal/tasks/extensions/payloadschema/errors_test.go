package payloadschema

import (
	"errors"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// violationsOf validates payload against schema and returns the flattened
// violations.
func violationsOf(t *testing.T, schema, payload string) []Violation {
	t.Helper()
	compiled, err := compile([]byte(schema))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	err = compiled.Validate(decode(t, payload))
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("Validate() = %v; want a validation error", err)
	}
	return violations(invalid)
}

func TestViolations(t *testing.T) {
	const schema = `{
		"type": "object",
		"properties": {
			"outcome": {"type": "string", "oneOf": [{"const": "approve"}, {"const": "needs_more_info"}]},
			"weight": {"type": "number", "exclusiveMinimum": 0},
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {"qty": {"type": "integer"}},
					"required": ["qty", "unit"]
				}
			},
			"a/b": {"type": "string"},
			"shape": {"oneOf": [{"type": "string"}, {"type": "number"}]}
		},
		"required": ["outcome", "reason"]
	}`

	tests := []struct {
		name    string
		payload string
		want    []Violation
	}{
		{
			name:    "missing properties are reported at each property",
			payload: `{}`,
			want: []Violation{
				{Path: "/outcome", Message: "missing property"},
				{Path: "/reason", Message: "missing property"},
			},
		},
		{
			name:    "a choice list names the allowed values once",
			payload: `{"outcome": "banana", "reason": "x"}`,
			want:    []Violation{{Path: "/outcome", Message: `value must be one of "approve", "needs_more_info"`}},
		},
		{
			name:    "other alternatives keep the library's message",
			payload: `{"outcome": "approve", "reason": "x", "shape": true}`,
			want:    []Violation{{Path: "/shape", Message: "'oneOf' failed, none matched"}},
		},
		{
			name:    "nested values are reported at their path",
			payload: `{"outcome": "approve", "reason": "x", "items": [{"qty": 1.5}]}`,
			want: []Violation{
				{Path: "/items/0/unit", Message: "missing property"},
				{Path: "/items/0/qty", Message: "got number, want integer"},
			},
		},
		{
			name:    "pointer tokens are escaped",
			payload: `{"outcome": "approve", "reason": "x", "a/b": 1}`,
			want:    []Violation{{Path: "/a~1b", Message: "got number, want string"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := violationsOf(t, schema, tt.payload)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("violations =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestInvalidPayloadError(t *testing.T) {
	err := &InvalidPayloadError{Violations: []Violation{
		{Path: "/outcome", Message: "missing property"},
		{Path: "/weight", Message: "must be > 0"},
	}}
	if !errors.Is(err, ErrInvalidPayload) {
		t.Error("errors.Is(err, ErrInvalidPayload) = false")
	}
	const want = "payload schema: invalid payload: /outcome: missing property; /weight: must be > 0"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q; want %q", got, want)
	}
}
