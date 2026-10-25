package taskconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskConfig_ValidateFiles(t *testing.T) {
	base := TaskConfig{SchemaVersion: CurrentSchemaVersion, TaskCode: "verify", Workflow: "verify_wf"}
	tests := []struct {
		name    string
		files   []string
		wantErr bool
	}{
		{"none", nil, false},
		{"paths", []string{"invoice", "documents[*].file"}, false},
		{"a path that does not parse", []string{"documents[0].file"}, true},
		{"a path twice", []string{"invoice", "invoice"}, true},
		{"an empty path", []string{""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			c.Files = tt.files
			err := c.Validate()
			if tt.wantErr {
				assert.ErrorContains(t, err, `"verify"`)
				return
			}
			assert.NoError(t, err)
		})
	}
}
