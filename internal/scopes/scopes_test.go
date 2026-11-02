package scopes

import (
	"strings"
	"testing"
)

func TestResolver_Of(t *testing.T) {
	r := NewResolver(map[string]string{"consignmentFlowWrite": "consignment:flow:write"})

	if got, ok := r.Of(ConsignmentFlowWrite); !ok || got != "consignment:flow:write" {
		t.Errorf("Of(ConsignmentFlowWrite) = %q, %v; want %q, true", got, ok, "consignment:flow:write")
	}
	if got, ok := r.Of(ConsignmentRead); ok {
		t.Errorf("Of(ConsignmentRead) = %q, true; want none: catalog.scopes leaves it out", got)
	}
}

func TestValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		entries map[string]string
		wantErr string
	}{
		"none":             {nil, ""},
		"a scope":          {map[string]string{"taskWrite": "task:write"}, ""},
		"an unknown key":   {map[string]string{"consignmentRaed": "consignment:read"}, `scopes["consignmentRaed"] is not a scope`},
		"an empty scope":   {map[string]string{"taskRead": ""}, `scopes["taskRead"] must map to one scope`},
		"two scopes":       {map[string]string{"taskRead": "task:read task:write"}, `scopes["taskRead"] must map to one scope`},
		"a trailing space": {map[string]string{"taskRead": "task:read "}, `scopes["taskRead"] must map to one scope`},
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate(tc.entries)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() err = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}
