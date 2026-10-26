package bootstrap

import (
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
)

// nonRetryable reports how Temporal classifies err when an activity returns it.
func nonRetryable(err error) bool {
	return temporal.GetDefaultFailureConverter().ErrorToFailure(err).GetApplicationFailureInfo().GetNonRetryable()
}

func TestSplitDefinitionLoader(t *testing.T) {
	reg := artifact.NewRegistry(testutil.MemLoader{
		"wf_branch.json":  []byte(`{"id": "branch", "name": "Branch", "version": 1, "nodes": []}`),
		"wf_invalid.json": []byte(`{`),
	})
	reg.RegisterArtifact("branch", "workflow", "", "wf_branch.json")
	reg.RegisterArtifact("invalid", "workflow", "", "wf_invalid.json")
	load := splitDefinitionLoader(reg)

	def, err := load("branch")
	require.NoError(t, err)
	assert.Equal(t, "branch", def.ID)

	// A workflow that isn't there, or doesn't parse, won't load on a retry either.
	for _, id := range []string{"missing", "invalid"} {
		_, err := load(id)
		require.Error(t, err)
		assert.True(t, nonRetryable(err), "%s: err = %v", id, err)
	}
}
