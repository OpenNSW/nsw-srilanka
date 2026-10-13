package consignment

import (
	"fmt"
	"strings"
)

// Config is what a TNSW deployment sets for consignments.
type Config struct {
	// Flows are the flows a trader or CHA may start on a consignment, besides the export
	// flow that creating it starts (POST /api/v1/consignments/{id}/flows). Optional: with
	// none, no flow can be started that way.
	Flows []FlowOption `yaml:"flows"`
}

// FlowOption is one flow a trader or CHA may start on a consignment.
type FlowOption struct {
	// TemplateID is the workflow definition the flow runs.
	TemplateID string `yaml:"templateId" json:"templateId"`
	// Label is what the portal's button for the flow says.
	Label string `yaml:"label" json:"label"`
}

// Validate checks every flow names a template and a label, and no template twice.
func (c Config) Validate() error {
	seen := make(map[string]bool, len(c.Flows))
	for i, f := range c.Flows {
		if strings.TrimSpace(f.TemplateID) == "" {
			return fmt.Errorf("flows[%d]: templateId is required", i)
		}
		if strings.TrimSpace(f.Label) == "" {
			return fmt.Errorf("flows[%d] (%s): label is required", i, f.TemplateID)
		}
		if seen[f.TemplateID] {
			return fmt.Errorf("flows[%d]: template %q is listed twice", i, f.TemplateID)
		}
		seen[f.TemplateID] = true
	}
	return nil
}
