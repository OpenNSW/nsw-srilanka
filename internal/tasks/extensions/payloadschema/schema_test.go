package payloadschema

import (
	"encoding/json"
	"testing"
)

// testSchema exercises what the forms use: required fields, types, an enum
// written as oneOf/const, a conditional requirement, and formats, including
// "file", which neither library defines.
const testSchema = `{
	"type": "object",
	"properties": {
		"outcome": {"type": "string", "oneOf": [{"const": "approve"}, {"const": "needs_more_info"}]},
		"reason": {"type": "string"},
		"weight": {"type": "number", "exclusiveMinimum": 0},
		"inspection_date": {"type": "string", "format": "date"},
		"inspection_time": {"type": "string", "format": "time"},
		"email": {"type": "string", "format": "email"},
		"invoice": {"type": "string", "format": "file"},
		"courier": {"type": "string"},
		"tracking_number": {"type": "string"}
	},
	"required": ["outcome"],
	"if": {"properties": {"outcome": {"const": "needs_more_info"}}},
	"then": {"required": ["reason"]},
	"dependencies": {"courier": ["tracking_number"]}
}`

// decode decodes payload as the task handler does, with encoding/json, so
// numbers arrive as float64.
func decode(t *testing.T, payload string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return m
}

func TestCompile_Validate(t *testing.T) {
	compiled, err := compile([]byte(testSchema))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{name: "valid", payload: `{"outcome": "approve", "weight": 1.5}`},
		{name: "missing required field", payload: `{"weight": 1.5}`, wantErr: true},
		{name: "empty string satisfies required", payload: `{"outcome": "approve", "reason": ""}`},
		{name: "wrong type", payload: `{"outcome": "approve", "weight": "heavy"}`, wantErr: true},
		{name: "value outside oneOf", payload: `{"outcome": "banana"}`, wantErr: true},
		{name: "number out of range", payload: `{"outcome": "approve", "weight": 0}`, wantErr: true},
		{name: "conditional requirement met", payload: `{"outcome": "needs_more_info", "reason": "x"}`},
		{name: "conditional requirement missing", payload: `{"outcome": "needs_more_info"}`, wantErr: true},
		{name: "undeclared field accepted", payload: `{"outcome": "approve", "extra": 1}`},
		{name: "valid date", payload: `{"outcome": "approve", "inspection_date": "2026-02-28"}`},
		{name: "impossible date", payload: `{"outcome": "approve", "inspection_date": "2026-02-30"}`, wantErr: true},
		{name: "time without timezone, as the trader app sends", payload: `{"outcome": "approve", "inspection_time": "10:00:00"}`},
		{name: "time with timezone", payload: `{"outcome": "approve", "inspection_time": "10:00:00+05:30"}`},
		{name: "time out of range", payload: `{"outcome": "approve", "inspection_time": "24:00:00"}`, wantErr: true},
		{name: "time without seconds", payload: `{"outcome": "approve", "inspection_time": "10:00"}`, wantErr: true},
		{name: "valid email", payload: `{"outcome": "approve", "email": "trader@example.lk"}`},
		{name: "email without a dot in the domain", payload: `{"outcome": "approve", "email": "trader@localhost"}`, wantErr: true},
		{name: "unknown format ignored", payload: `{"outcome": "approve", "invoice": "anything"}`},
		{name: "draft-07 dependencies applied", payload: `{"outcome": "approve", "courier": "DHL"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := compiled.Validate(decode(t, tt.payload))
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompile_RejectsBadSchemas(t *testing.T) {
	tests := []struct {
		name   string
		schema string
	}{
		{name: "not JSON", schema: `{"type": `},
		{name: "not a valid schema", schema: `{"type": 5}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := compile([]byte(tt.schema)); err == nil {
				t.Fatal("compile() succeeded; want an error")
			}
		})
	}
}

func TestAjvFormats_IgnoreNonStrings(t *testing.T) {
	for _, f := range ajvFormats {
		if err := f.Validate(float64(10)); err != nil {
			t.Errorf("%s: Validate(10) = %v; want nil, formats apply to strings only", f.Name, err)
		}
	}
}

func TestValidateTime(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"00:00:00", true},
		{"23:59:59", true},
		{"23:59:60", true},
		{"10:00:00.123", true},
		{"10:00:00Z", true},
		{"10:00:00z", true},
		{"10:00:00-0530", true},
		{"22:59:60", false},
		{"10:60:00", false},
		{"1:00:00", false},
		{"10:00:00+5", false},
		{"noon", false},
	}
	for _, tt := range tests {
		if err := validateTime(tt.value); (err == nil) != tt.valid {
			t.Errorf("validateTime(%q) = %v; want valid %v", tt.value, err, tt.valid)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"trader@example.lk", true},
		{"first.last+tag@mail.example.com", true},
		{"TRADER@EXAMPLE.LK", true},
		{"trader@localhost", false},
		{"trader@-example.lk", false},
		{".trader@example.lk", false},
		{"trader@@example.lk", false},
		{"trader", false},
	}
	for _, tt := range tests {
		if err := validateEmail(tt.value); (err == nil) != tt.valid {
			t.Errorf("validateEmail(%q) = %v; want valid %v", tt.value, err, tt.valid)
		}
	}
}
