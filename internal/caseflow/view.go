package caseflow

import (
	"context"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
)

// View is one flow (top-level workflow) of a case as the API presents it: a consignment's,
// or an agency case's in its admin view. An admin opens its engine status by WorkflowID.
type View struct {
	WorkflowID   string `json:"workflowId"`
	TemplateID   string `json:"templateId"`
	TemplateName string `json:"templateName"` // the workflow definition's name, else TemplateID
	Status       string `json:"status"`       // STARTING, STARTED or COMPLETED
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

// ViewOf presents f, named by its workflow definition's name in the manifest version the
// flow runs on (its template ID when the definition has none or cannot be loaded).
func ViewOf(ctx context.Context, artifacts *artifact.Set, f Flow) View {
	name := f.TemplateID
	if reg, err := artifacts.Registry(ctx, f.ManifestVersion); err == nil {
		if def, err := workflowdef.Load(ctx, reg, f.TemplateID); err == nil && def.Name != "" {
			name = def.Name
		}
	}
	return View{
		WorkflowID:   f.WorkflowID,
		TemplateID:   f.TemplateID,
		TemplateName: name,
		Status:       string(f.Status),
		CreatedAt:    f.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    f.UpdatedAt.Format(time.RFC3339),
	}
}
