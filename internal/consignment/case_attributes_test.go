package consignment

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/OpenNSW/core/json/jsonpointer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

// Every key caseAttributes writes is shared: one that is not would never reach an agency.
func TestCaseAttributesAreShared(t *testing.T) {
	for key := range caseAttributes(&company.Record{Name: "Adam PVT LTD"}) {
		assert.True(t, slices.Contains(SharedCaseAttributes, key), "key %q is not in SharedCaseAttributes", key)
	}
}

// Each agency config's caseColumns resolves against what TNSW sends, so a change on either
// side that would stop case names appearing fails here.
func TestAgencyCaseColumnsResolveAgainstCaseAttributes(t *testing.T) {
	sent := caseAttributes(&company.Record{Name: "Adam PVT LTD"})
	files, err := filepath.Glob(filepath.Join("..", "..", "configs", "agency", "*", "config*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		t.Run(filepath.ToSlash(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			var cfg struct {
				Flows caseflow.Config `yaml:"flows"`
			}
			require.NoError(t, yaml.Unmarshal(raw, &cfg))
			pointer, ok := cfg.Flows.Clients.CaseColumns[caseflow.CaseColumnName]
			require.True(t, ok, "flows.clients.caseColumns sets no %s", caseflow.CaseColumnName)
			name, _ := jsonpointer.Get(sent, pointer)
			assert.Equal(t, "Adam PVT LTD", name)
		})
	}
}
