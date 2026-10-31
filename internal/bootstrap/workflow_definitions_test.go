package bootstrap

import (
	"context"
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

func TestWorkflowDefinitionLoader(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"wf_branch.json":   []byte(`{"id": "branch", "name": "Branch", "version": 1, "nodes": []}`),
		"wf_invalid.json":  []byte(`{`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"branch","kind":"workflow","path":"wf_branch.json"},{"id":"invalid","kind":"workflow","path":"wf_invalid.json"}]}`),
	})
	require.NoError(t, err)
	load := workflowDefinitionLoader(set)

	def, err := load(1, "branch")
	require.NoError(t, err)
	assert.Equal(t, "branch", def.ID)

	// A workflow that isn't there, or doesn't parse, won't load on a retry either.
	for _, id := range []string{"missing", "invalid"} {
		_, err := load(1, id)
		require.Error(t, err)
		assert.True(t, nonRetryable(err), "%s: err = %v", id, err)
	}
}
