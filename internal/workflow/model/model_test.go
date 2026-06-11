package model

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestUUIDArray_MarshalJSON(t *testing.T) {
	u1 := uuid.NewString()
	u2 := uuid.NewString()

	tests := []struct {
		name     string
		input    StringArray
		expected string
	}{
		{
			name:     "Populated",
			input:    StringArray{u1, u2},
			expected: `["` + u1 + `","` + u2 + `"]`,
		},
		{
			name:     "Empty",
			input:    StringArray{},
			expected: `[]`,
		},
		{
			name:     "Nil",
			input:    nil,
			expected: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := json.Marshal(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestUUIDArray_UnmarshalJSON(t *testing.T) {
	u1 := uuid.NewString()
	u2 := uuid.NewString()

	tests := []struct {
		name      string
		input     string
		expectErr bool
		expected  StringArray
	}{
		{
			name:      "Valid Populated",
			input:     `["` + u1 + `","` + u2 + `"]`,
			expectErr: false,
			expected:  StringArray{u1, u2},
		},
		{
			name:      "Valid Empty",
			input:     `[]`,
			expectErr: false,
			expected:  StringArray{},
		},
		{
			name:      "Null",
			input:     `null`,
			expectErr: false,
			expected:  StringArray{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result StringArray
			err := json.Unmarshal([]byte(tt.input), &result)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestModel_TableNames(t *testing.T) {
	assert.Equal(t, "workflow_templates", (&WorkflowTemplate{}).TableName())
	assert.Equal(t, "workflow_node_templates", (&WorkflowNodeTemplate{}).TableName())
	assert.Equal(t, "workflow_nodes", (&WorkflowNode{}).TableName())
}

func TestModel_WorkflowTemplate_GetNodeTemplateIDs(t *testing.T) {
	id1 := uuid.NewString()
	id2 := uuid.NewString()
	wt := &WorkflowTemplate{
		NodeTemplates: StringArray{id1, id2},
	}
	ids := wt.GetNodeTemplateIDs()
	assert.Len(t, ids, 2)
	assert.ElementsMatch(t, []string{id1, id2}, ids)
}

func TestModel_BaseHooks(t *testing.T) {
	base := &BaseModel{}
	err := base.BeforeCreate(nil)
	assert.NoError(t, err)
	assert.NotNil(t, base.ID)
	assert.False(t, base.CreatedAt.IsZero())
	assert.False(t, base.UpdatedAt.IsZero())

	err = base.BeforeUpdate(nil)
	assert.NoError(t, err)
}
