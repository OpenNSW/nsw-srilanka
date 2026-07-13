package trade

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
)

func TestHscodeSplitBuilderFunc(t *testing.T) {
	config := json.RawMessage(`{
		"hs_code_flows": {
			"0801.11.90": ["cda-certificate-reg"],
			"0801.12.00": ["cda-certificate-reg", "customs-workflow"]
		}
	}`)

	tests := []struct {
		name          string
		config        json.RawMessage
		hsCodes       []any
		traderCompany any
		want          []map[string]any
	}{
		{
			name:    "HS code expands to its configured flow",
			config:  config,
			hsCodes: []any{"0801.11.90"},
			want: []map[string]any{
				{"template_id": "cda-certificate-reg", "branch_id": "cda-certificate-reg", "payload": map[string]any{"hs_codes": []string{"0801.11.90"}}},
			},
		},
		{
			name:    "unmapped value passes through as template ID",
			config:  config,
			hsCodes: []any{"fcau-health-certificate-reg"},
			want: []map[string]any{
				{"template_id": "fcau-health-certificate-reg", "branch_id": "fcau-health-certificate-reg", "payload": map[string]any{}},
			},
		},
		{
			name:    "flows deduplicated across HS codes, payload collects triggering codes",
			config:  config,
			hsCodes: []any{"0801.11.90", "0801.12.00", "npqs-export-phytosanitary-reg"},
			want: []map[string]any{
				{"template_id": "cda-certificate-reg", "branch_id": "cda-certificate-reg", "payload": map[string]any{"hs_codes": []string{"0801.11.90", "0801.12.00"}}},
				{"template_id": "customs-workflow", "branch_id": "customs-workflow", "payload": map[string]any{"hs_codes": []string{"0801.12.00"}}},
				{"template_id": "npqs-export-phytosanitary-reg", "branch_id": "npqs-export-phytosanitary-reg", "payload": map[string]any{}},
			},
		},
		{
			name: "shared_payload merged into every split item",
			config: json.RawMessage(`{
				"hs_code_flows": {"0801.11.90": ["cda-certificate-reg"]},
				"shared_payload": {"cusdec_signal": "cusdec_ready"}
			}`),
			hsCodes: []any{"0801.11.90", "customs-workflow"},
			want: []map[string]any{
				{"template_id": "cda-certificate-reg", "branch_id": "cda-certificate-reg", "payload": map[string]any{"cusdec_signal": "cusdec_ready", "hs_codes": []string{"0801.11.90"}}},
				{"template_id": "customs-workflow", "branch_id": "customs-workflow", "payload": map[string]any{"cusdec_signal": "cusdec_ready"}},
			},
		},
		{
			name: "trader_company input copied into every split item payload",
			config: json.RawMessage(`{
				"hs_code_flows": {"0801.11.90": ["cda-certificate-reg"]}
			}`),
			hsCodes:       []any{"0801.11.90", "customs-workflow"},
			traderCompany: map[string]any{"name": "ADAM PVT LTD"},
			want: []map[string]any{
				{"template_id": "cda-certificate-reg", "branch_id": "cda-certificate-reg", "payload": map[string]any{"trader_company": map[string]any{"name": "ADAM PVT LTD"}, "hs_codes": []string{"0801.11.90"}}},
				{"template_id": "customs-workflow", "branch_id": "customs-workflow", "payload": map[string]any{"trader_company": map[string]any{"name": "ADAM PVT LTD"}}},
			},
		},
		{
			name:    "no config keeps legacy passthrough behaviour",
			config:  nil,
			hsCodes: []any{"sltb-blendsheet-approval"},
			want: []map[string]any{
				{"template_id": "sltb-blendsheet-approval", "branch_id": "sltb-blendsheet-approval", "payload": map[string]any{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputs := map[string]any{"hs_codes": tt.hsCodes}
			if tt.traderCompany != nil {
				inputs["trader_company"] = tt.traderCompany
			}
			ctx := plugins.PluginContext{
				Inputs: inputs,
				Record: &store.TaskRecord{},
			}
			if err := HscodeSplitBuilderFunc(ctx, tt.config); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := ctx.Record.Data["split_items"]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("split_items mismatch\n got: %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func TestHscodeSplitBuilderFuncErrors(t *testing.T) {
	tests := []struct {
		name   string
		config json.RawMessage
		inputs map[string]any
	}{
		{name: "missing hs_codes input", inputs: map[string]any{}},
		{name: "hs_codes not an array", inputs: map[string]any{"hs_codes": "0801.11.90"}},
		{name: "non-string item", inputs: map[string]any{"hs_codes": []any{42}}},
		{name: "invalid plugin_properties", config: json.RawMessage(`{`), inputs: map[string]any{"hs_codes": []any{"0801.11.90"}}},
		{
			name:   "HS code resolving to no flows",
			config: json.RawMessage(`{"hs_code_flows": {"0801.11.90": []}}`),
			inputs: map[string]any{"hs_codes": []any{"0801.11.90"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := plugins.PluginContext{Inputs: tt.inputs, Record: &store.TaskRecord{}}
			if err := HscodeSplitBuilderFunc(ctx, tt.config); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
