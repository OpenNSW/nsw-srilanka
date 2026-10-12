package notify

import (
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"github.com/OpenNSW/core/artifact/adapter/types"
	"github.com/OpenNSW/core/taskflow/extensions"
)

// Extension ids. These must match the ExtensionConfig.id values declared in the
// SubTaskTemplate JSON configs loaded into the artifact registry.
const (
	ExtNotification = "notification"
)

// Register installs the nsw-srilanka task extensions on reg. The notification
// extension dispatches SMS/email as a side-effect of a step completing. s and
// loader must be non-nil; loader resolves template_id documents.
func Register(reg *extensions.Registry, s sender, loader templateLoader) error {
	if reg == nil {
		return fmt.Errorf("extensions: registry is nil")
	}
	if s == nil {
		return fmt.Errorf("extensions: sender is nil")
	}
	if loader == nil {
		return fmt.Errorf("extensions: template loader is nil")
	}

	if err := reg.Register(ExtNotification, NewNotificationExtension(s, loader)); err != nil {
		return fmt.Errorf("extensions: register %s: %w", ExtNotification, err)
	}
	return nil
}

// TemplateRefs returns the generic templates a step template's notification extensions name
// (template_id), for artifact.Set.Validate: a manifest that lists the step without one of them
// fails its notification at run time. raw must already be a valid step template.
func TemplateRefs(raw []byte) ([]artifact.Ref, error) {
	var step types.StepTemplate
	if err := json.Unmarshal(raw, &step); err != nil {
		return nil, fmt.Errorf("decode step template: %w", err)
	}
	var refs []artifact.Ref
	for i, ext := range step.Extensions {
		if ext.ID != ExtNotification || len(ext.Properties) == 0 {
			continue
		}
		var cfg notificationConfig
		if err := json.Unmarshal(ext.Properties, &cfg); err != nil {
			return nil, fmt.Errorf("step template %q: extension %d (%s): invalid properties: %w", step.ID, i, ext.ID, err)
		}
		if cfg.TemplateID != "" {
			refs = append(refs, artifact.Ref{ID: cfg.TemplateID, Kinds: []artifact.Kind{generictemplate.Kind}, Field: fmt.Sprintf("extension %d (%s) template_id", i, ext.ID)})
		}
	}
	return refs, nil
}
