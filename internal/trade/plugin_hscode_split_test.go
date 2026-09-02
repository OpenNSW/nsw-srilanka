package trade

import (
	"encoding/json"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContext(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Inputs: inputs,
		Record: &store.TaskRecord{
			Data: make(map[string]any),
		},
	}
}

func TestHscodeSplitBuilderFunc_Success(t *testing.T) {
	config := []byte(`{
		"mappings": {
			"0902.30.11": [
				"sltb-blendsheet-approval",
				"fcau-health-certificate-reg",
				"npqs-export-phytosanitary-reg",
				"customs-workflow"
			]
		}
	}`)

	ctx := newTestContext(map[string]any{
		"hs_codes": []any{"0902.30.11"},
		"company": map[string]any{
			"id":   "adam-pvt-ltd",
			"name": "ADAM PVT LTD",
		},
	})

	err := HscodeSplitBuilderFunc(ctx, config)
	require.NoError(t, err)

	splitItems, ok := ctx.Record.Data["split_items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, splitItems, 4)

	expectedWorkflows := []string{
		"sltb-blendsheet-approval",
		"fcau-health-certificate-reg",
		"npqs-export-phytosanitary-reg",
		"customs-workflow",
	}

	for i, expected := range expectedWorkflows {
		assert.Equal(t, expected, splitItems[i]["template_id"])
		assert.Equal(t, expected, splitItems[i]["branch_id"])
		payload, ok := splitItems[i]["payload"].(map[string]any)
		require.True(t, ok)
		company, ok := payload["company"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "adam-pvt-ltd", company["id"])
	}
}

func TestHscodeSplitBuilderFunc_Deduplication(t *testing.T) {
	config := []byte(`{
		"mappings": {
			"0902.30.11": ["fcau-health-certificate-reg", "customs-workflow"],
			"0902.10.11": ["npqs-export-phytosanitary-reg", "customs-workflow"]
		}
	}`)

	ctx := newTestContext(map[string]any{
		"hs_codes": []any{"0902.30.11", "0902.10.11"},
	})

	err := HscodeSplitBuilderFunc(ctx, config)
	require.NoError(t, err)

	splitItems, ok := ctx.Record.Data["split_items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, splitItems, 3)

	assert.Equal(t, "fcau-health-certificate-reg", splitItems[0]["template_id"])
	assert.Equal(t, "customs-workflow", splitItems[1]["template_id"])
	assert.Equal(t, "npqs-export-phytosanitary-reg", splitItems[2]["template_id"])
}

func TestHscodeSplitBuilderFunc_UnmappedCodeFails(t *testing.T) {
	config := []byte(`{
		"mappings": {
			"0902.30.11": ["fcau-health-certificate-reg"]
		}
	}`)

	ctx := newTestContext(map[string]any{
		"hs_codes": []any{"unknown-code"},
	})

	err := HscodeSplitBuilderFunc(ctx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unmapped HS code "unknown-code"`)
}

func TestHscodeSplitBuilderFunc_InvalidInputs(t *testing.T) {
	config := []byte(`{
		"mappings": {
			"0902.30.11": ["fcau-health-certificate-reg"]
		}
	}`)

	tests := []struct {
		name        string
		inputs      map[string]any
		config      json.RawMessage
		expectedErr string
	}{
		{
			name:        "missing config",
			inputs:      map[string]any{"hs_codes": []any{"0902.30.11"}},
			config:      nil,
			expectedErr: "config is required",
		},
		{
			name:        "empty mappings in config",
			inputs:      map[string]any{"hs_codes": []any{"0902.30.11"}},
			config:      json.RawMessage(`{"mappings":{}}`),
			expectedErr: "no mappings configured",
		},
		{
			name:        "missing hs_codes key",
			inputs:      map[string]any{},
			config:      config,
			expectedErr: "hs_codes not found in inputs",
		},
		{
			name: "hs_codes is not an array",
			inputs: map[string]any{
				"hs_codes": "0902.30.11",
			},
			config:      config,
			expectedErr: "hs_codes is not an array",
		},
		{
			name: "hs_codes is empty",
			inputs: map[string]any{
				"hs_codes": []any{},
			},
			config:      config,
			expectedErr: "hs_codes cannot be empty",
		},
		{
			name: "hs_codes element is not a string",
			inputs: map[string]any{
				"hs_codes": []any{12345},
			},
			config:      config,
			expectedErr: "item[0] is not a string",
		},
		{
			name: "invalid config json",
			inputs: map[string]any{
				"hs_codes": []any{"0902.30.11"},
			},
			config:      json.RawMessage(`{malformed-json`),
			expectedErr: "invalid config",
		},
		{
			name:        "empty workflow template ID in mappings",
			inputs:      map[string]any{"hs_codes": []any{"0902.30.11"}},
			config:      json.RawMessage(`{"mappings":{"0902.30.11":[""]}}`),
			expectedErr: "has an empty workflow template ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestContext(tc.inputs)
			err := HscodeSplitBuilderFunc(ctx, tc.config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErr)
		})
	}
}
