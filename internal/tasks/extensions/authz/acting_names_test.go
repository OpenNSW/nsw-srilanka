package authz

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/OpenNSW/core/artifact/adapter/types"
)

func authzBlock(phase types.ExecutionPhase, properties string) types.ExtensionConfig {
	ext := types.ExtensionConfig{ID: ExtAuthz, Phase: phase}
	if properties != "" {
		ext.Properties = json.RawMessage(properties)
	}
	return ext
}

func TestActingNames(t *testing.T) {
	tests := []struct {
		name         string
		exts         []types.ExtensionConfig
		wantNames    []string
		wantDeclared bool
		wantErr      bool
	}{
		{name: "no extensions"},
		{name: "another extension only", exts: []types.ExtensionConfig{{ID: "notify", Phase: types.PhasePreResume}}},
		{
			// Core runs a block only in its own phase, and only PRE_RESUME gates a write.
			name: "authz in another phase gates nothing",
			exts: []types.ExtensionConfig{authzBlock(types.ExecutionPhase("POST_RESUME"), `{"S":{"submit":["officer"]}}`)},
		},
		{
			name:         "names across states and commands, sorted and unique",
			exts:         []types.ExtensionConfig{authzBlock(types.PhasePreResume, `{"A":{"submit":["officer","fcau"]},"B":{"approve":["reviewer","officer"]}}`)},
			wantNames:    []string{"fcau", "officer", "reviewer"},
			wantDeclared: true,
		},
		{
			name:         "declared without properties: nobody may act",
			exts:         []types.ExtensionConfig{authzBlock(types.PhasePreResume, "")},
			wantDeclared: true,
		},
		{
			name:         "declared with an empty rule: nobody may act",
			exts:         []types.ExtensionConfig{authzBlock(types.PhasePreResume, `{}`)},
			wantDeclared: true,
		},
		{
			// A submission must pass every block, so only names they all allow may act.
			name: "several blocks: the names every block allows",
			exts: []types.ExtensionConfig{
				authzBlock(types.PhasePreResume, `{"S":{"submit":["officer","reviewer"]}}`),
				authzBlock(types.PhasePreResume, `{"S":{"submit":["reviewer","fcau"]}}`),
			},
			wantNames:    []string{"reviewer"},
			wantDeclared: true,
		},
		{
			name:         "malformed rule is an error, not a pass",
			exts:         []types.ExtensionConfig{authzBlock(types.PhasePreResume, `{"S":["officer"]}`)},
			wantDeclared: true,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, declared, err := ActingNames(tt.exts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if declared != tt.wantDeclared {
				t.Errorf("declared = %v, want %v", declared, tt.wantDeclared)
			}
			if !reflect.DeepEqual(names, tt.wantNames) {
				t.Errorf("names = %v, want %v", names, tt.wantNames)
			}
		})
	}
}
