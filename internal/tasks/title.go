package tasks

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/taskflow/renderconfig"
	"github.com/OpenNSW/core/taskflow/store"
)

// RenderConfigs resolves a task's render config through its task template, in the
// manifest version the task runs on.
func RenderConfigs(artifacts *artifact.Set) RenderConfigOf {
	return func(ctx context.Context, record store.TaskRecord) (json.RawMessage, error) {
		return renderconfig.Load(ctx, artifacts, record)
	}
}

// Title names record's task, wherever a task is shown: the title at the root of its render
// config, else its task template's id. A render config that doesn't resolve only costs the
// name, so it is logged; without renderConfigOf, the task is named by its template.
func Title(ctx context.Context, renderConfigOf RenderConfigOf, record store.TaskRecord) string {
	if renderConfigOf == nil {
		return record.TaskTemplateID
	}
	rc, err := renderConfigOf(ctx, record)
	if err != nil {
		slog.WarnContext(ctx, "tasks: render config unavailable for the task's title", "taskId", record.TaskID, "error", err)
		return record.TaskTemplateID
	}
	return titleOf(rc, record.TaskTemplateID)
}

// titleOf returns the title at the root of renderConfig, or fallback when it has none.
func titleOf(renderConfig json.RawMessage, fallback string) string {
	if len(renderConfig) > 0 {
		var rc struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(renderConfig, &rc); err == nil && rc.Title != "" {
			return rc.Title
		}
	}
	return fallback
}
