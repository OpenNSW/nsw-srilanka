package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/tasktemplate"
	"github.com/OpenNSW/core/taskflow/store"
)

// errNoTaskTemplate means a task record names no task template.
var errNoTaskTemplate = errors.New("task has no task template")

// TaskTemplateOf returns record's task template as raw JSON, in the manifest version
// it runs on. Its read policy decides who may read the task (see readauthz).
type TaskTemplateOf func(ctx context.Context, record store.TaskRecord) (json.RawMessage, error)

// rawTaskTemplate is a task template kept as its raw JSON, for the fields core's
// types.TaskTemplate does not carry.
type rawTaskTemplate struct {
	raw json.RawMessage
}

func (rawTaskTemplate) Kind() artifact.Kind { return tasktemplate.Kind }

func (t *rawTaskTemplate) Parse(raw []byte) error {
	if !json.Valid(raw) {
		return errors.New("decode task template: invalid JSON")
	}
	t.raw = raw
	return nil
}

// TaskTemplates returns a TaskTemplateOf that reads task templates from artifacts.
func TaskTemplates(artifacts *artifact.Set) TaskTemplateOf {
	return func(ctx context.Context, record store.TaskRecord) (json.RawMessage, error) {
		if record.TaskTemplateID == "" {
			return nil, fmt.Errorf("task %s: %w", record.TaskID, errNoTaskTemplate)
		}
		reg, err := artifacts.Registry(ctx, record.ManifestVersion)
		if err != nil {
			return nil, fmt.Errorf("task %s: task template %q: %w", record.TaskID, record.TaskTemplateID, err)
		}
		template, err := artifact.Latest[rawTaskTemplate](ctx, reg, record.TaskTemplateID)
		if err != nil {
			return nil, fmt.Errorf("task %s: task template %q in manifest version %d: %w", record.TaskID, record.TaskTemplateID, record.ManifestVersion, err)
		}
		return template.raw, nil
	}
}
