// Package taskconfigart registers taskconfig.TaskConfig as the "task_config" kind of
// the core artifact registry.
package taskconfigart

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/artifact"

	"github.com/OpenNSW/nsw-srilanka/internal/agency/taskconfig"
)

// Kind is the manifest kind of a task config artifact.
const Kind artifact.Kind = "task_config"

type loadable struct {
	taskconfig.TaskConfig
}

func (loadable) Kind() artifact.Kind { return Kind }

func (l *loadable) Parse(raw []byte) error {
	if err := json.Unmarshal(raw, &l.TaskConfig); err != nil {
		return fmt.Errorf("parse task config: %w", err)
	}
	return l.Validate()
}

// Load returns the task config registered under taskCode, parsed and validated. An
// unknown taskCode returns an error matching artifact.ErrNotFound.
func Load(ctx context.Context, reg *artifact.Registry, taskCode string) (*taskconfig.TaskConfig, error) {
	l, err := artifact.Latest[loadable](ctx, reg, taskCode)
	if err != nil {
		return nil, err
	}
	return &l.TaskConfig, nil
}
