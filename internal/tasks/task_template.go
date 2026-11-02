package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/tasktemplate"
	"github.com/OpenNSW/core/artifact/adapter/types"
	"github.com/OpenNSW/core/taskflow/store"
)

// errNoTaskTemplate means a task record names no task template.
var errNoTaskTemplate = errors.New("task has no task template")

// TaskTemplateOf returns record's task template, in the manifest version it runs on.
// Its read.roles decide who may read the task (see readauthz), and its claimable field
// whether the task has claims.
type TaskTemplateOf func(ctx context.Context, record store.TaskRecord) (types.TaskTemplate, error)

// TaskTemplates returns a TaskTemplateOf that reads task templates from artifacts.
func TaskTemplates(artifacts *artifact.Set) TaskTemplateOf {
	return func(ctx context.Context, record store.TaskRecord) (types.TaskTemplate, error) {
		if record.TaskTemplateID == "" {
			return types.TaskTemplate{}, fmt.Errorf("task %s: %w", record.TaskID, errNoTaskTemplate)
		}
		reg, err := artifacts.Registry(ctx, record.ManifestVersion)
		if err != nil {
			return types.TaskTemplate{}, fmt.Errorf("task %s: task template %q: %w", record.TaskID, record.TaskTemplateID, err)
		}
		template, err := tasktemplate.Load(ctx, reg, record.TaskTemplateID)
		if err != nil {
			return types.TaskTemplate{}, fmt.Errorf("task %s: task template %q in manifest version %d: %w", record.TaskID, record.TaskTemplateID, record.ManifestVersion, err)
		}
		return template, nil
	}
}
