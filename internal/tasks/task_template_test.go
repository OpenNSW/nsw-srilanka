package tasks

import (
	"context"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskTemplates(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"manifest_v1.json": []byte(`{"artifacts":[
			{"id":"t","kind":"task_template","path":"t.json"},
			{"id":"plain","kind":"task_template","path":"plain.json"},
			{"id":"bad","kind":"task_template","path":"bad.json"},
			{"id":"yes","kind":"task_template","path":"yes.json"}
		]}`),
		"t.json":     []byte(`{"id":"t","workflow_id":"t-flow","render_config_id":"t:render","claimable":true,"read":{"roles":["cha","trader"]}}`),
		"plain.json": []byte(`{"id":"plain","workflow_id":"p-flow","render_config_id":"p:render"}`),
		"bad.json":   []byte(`{"id":`),
		"yes.json":   []byte(`{"id":"yes","workflow_id":"y-flow","render_config_id":"y:render","claimable":"yes"}`),
	})
	require.NoError(t, err)
	of := TaskTemplates(set)
	ctx := context.Background()

	t.Run("returns the template the task names, with claimable and read.roles in order", func(t *testing.T) {
		template, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "t", ManifestVersion: 1})
		require.NoError(t, err)
		assert.Equal(t, "t-flow", template.WorkflowID)
		assert.True(t, template.Claimable)
		assert.Equal(t, []string{"cha", "trader"}, template.Read.Roles)
	})
	t.Run("a template without claimable or read.roles has no claims and no read roles", func(t *testing.T) {
		template, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "plain", ManifestVersion: 1})
		require.NoError(t, err)
		assert.False(t, template.Claimable)
		assert.Empty(t, template.Read.Roles)
	})
	t.Run("a task naming no template is an error", func(t *testing.T) {
		_, err := of(ctx, store.TaskRecord{TaskID: "1", ManifestVersion: 1})
		assert.ErrorIs(t, err, errNoTaskTemplate)
	})
	t.Run("a template that isn't JSON is an error", func(t *testing.T) {
		_, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "bad", ManifestVersion: 1})
		assert.Error(t, err)
	})
	// A claimable that isn't a boolean is an error, never read as true or false.
	t.Run("a template whose claimable is not a boolean is an error", func(t *testing.T) {
		_, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "yes", ManifestVersion: 1})
		assert.Error(t, err)
	})
	t.Run("a template missing from the manifest version is an error", func(t *testing.T) {
		_, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "missing", ManifestVersion: 1})
		assert.ErrorIs(t, err, artifact.ErrNotFound)
	})
}
