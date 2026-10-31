package tasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskTemplates(t *testing.T) {
	const template = `{"id":"t","workflow_id":"t-flow","render_config_id":"t:render","read":{"roles":["cha","trader"]}}`
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"manifest_v1.json": []byte(`{"artifacts":[
			{"id":"t","kind":"task_template","path":"t.json"},
			{"id":"bad","kind":"task_template","path":"bad.json"}
		]}`),
		"t.json":   []byte(template),
		"bad.json": []byte(`{"id":`),
	})
	require.NoError(t, err)
	of := TaskTemplates(set)
	ctx := context.Background()

	t.Run("returns the template the task names, as written", func(t *testing.T) {
		raw, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "t", ManifestVersion: 1})
		require.NoError(t, err)
		assert.JSONEq(t, template, string(raw))
	})
	t.Run("a task naming no template is an error", func(t *testing.T) {
		_, err := of(ctx, store.TaskRecord{TaskID: "1", ManifestVersion: 1})
		assert.ErrorIs(t, err, errNoTaskTemplate)
	})
	t.Run("a template that isn't JSON is an error", func(t *testing.T) {
		_, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "bad", ManifestVersion: 1})
		assert.Error(t, err)
	})
	t.Run("a template missing from the manifest version is an error", func(t *testing.T) {
		_, err := of(ctx, store.TaskRecord{TaskID: "1", TaskTemplateID: "missing", ManifestVersion: 1})
		assert.ErrorIs(t, err, artifact.ErrNotFound)
	})
}

func TestClaimable(t *testing.T) {
	for name, tt := range map[string]struct {
		template string
		want     bool
		wantErr  bool
	}{
		"claimable":         {`{"id":"t","claimable":true}`, true, false},
		"says not":          {`{"id":"t","claimable":false}`, false, false},
		"no claimable":      {`{"id":"t"}`, false, false},
		"not a boolean":     {`{"id":"t","claimable":"yes"}`, false, true},
		"null is not there": {`{"id":"t","claimable":null}`, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := claimable(json.RawMessage(tt.template))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
