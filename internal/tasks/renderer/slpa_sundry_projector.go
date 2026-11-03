package renderer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/uiprojector"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/serviceorder"
)

// ProjectorSLPASundry defines the sundry-order form projector.
const ProjectorSLPASundry uiprojector.ProjectorType = "SLPA_SUNDRY"

// SLPASundryProjector renders the sundry-order form with the sundry services the
// CMS offers as the list each line's service is picked from, and the
// consignment's containers as the list its container is picked from.
//
// The services are SLPA's and change on their side, so the step before the form
// asks the CMS for them; a schema authored ahead of time would list what SLPA
// offered when it was written. The containers are the consignment's, read from
// its tasks by an earlier step for the same reason.
type SLPASundryProjector struct{}

// NewSLPASundryProjector creates a new SLPASundryProjector.
func NewSLPASundryProjector() *SLPASundryProjector {
	return &SLPASundryProjector{}
}

// Type returns the projector type.
func (p *SLPASundryProjector) Type() uiprojector.ProjectorType {
	return ProjectorSLPASundry
}

// sundryServicesNamespace is where the lookup records what the CMS offers,
// sundryContainersNamespace where the case's containers are recorded, and
// sundryFormNamespace is what the trader's form edits.
const (
	sundryServicesNamespace   = "services"
	sundryContainersNamespace = "containers"
	sundryFormNamespace       = "sundryform"
)

// Project renders the form artifact with the service field of every line turned
// into a list of the sundry services the CMS offers, and its container field
// into a list of the consignment's containers.
//
// With nothing to offer a field is rendered as authored, as the consolidation
// form is: an empty list is a field nothing can satisfy. The step's own panel
// says why the list is missing and offers to look again.
func (p *SLPASundryProjector) Project(_ context.Context, templateContent []byte, data any) (uiprojector.Projection, error) {
	var form struct {
		Schema   map[string]any `json:"schema"`
		UISchema map[string]any `json:"uiSchema"`
	}
	if err := json.Unmarshal(templateContent, &form); err != nil {
		return uiprojector.Projection{}, fmt.Errorf("slpa_sundry_projector: parse form: %w", err)
	}

	record, _ := data.(map[string]any)
	if choices := sundryServices(record); len(choices) > 0 {
		offerServices(form.Schema, choices)
	}
	if choices := sundryContainers(record); len(choices) > 0 {
		offerContainers(form.Schema, choices)
	}

	return uiprojector.Projection{
		Type: uiprojector.SectionTypeForm,
		Content: uiprojector.FormContent{
			Schema:   form.Schema,
			UISchema: form.UISchema,
			Data:     record[sundryFormNamespace],
		},
	}, nil
}

// sundryServices reads the services the lookup recorded. An entry missing its ID
// or its name is left out: it would be an option no order could use, or one the
// trader could not read.
func sundryServices(record map[string]any) []choice {
	ns, _ := record[sundryServicesNamespace].(map[string]any)
	rows := fields.Rows(ns[serviceorder.ServicesKey])

	out := make([]choice, 0, len(rows))
	for _, row := range rows {
		id := fields.String(row, "service_id")
		name := fields.String(row, "service_name")
		if id == "" || name == "" {
			continue
		}
		out = append(out, choice{value: id, label: name})
	}
	return out
}

// sundryContainers reads the case's containers. The value is the dummy number,
// because that is what SLPA keys the parent order's containers by; the real
// container it is consolidated to, when it has one, is shown beside it so the
// trader can tell the containers apart.
func sundryContainers(record map[string]any) []choice {
	ns, _ := record[sundryContainersNamespace].(map[string]any)
	rows := fields.Rows(ns["containers"])

	out := make([]choice, 0, len(rows))
	for _, row := range rows {
		dummy := fields.String(row, "container_no")
		if dummy == "" {
			continue
		}
		label := dummy
		if realNo := fields.String(row, "real_container_no"); realNo != "" {
			label = dummy + " - " + realNo
		}
		out = append(out, choice{value: dummy, label: label})
	}
	return out
}

// offerServices replaces the service field of a line with the list, as a oneOf
// for the same reason the consolidation form uses one.
func offerServices(schema map[string]any, choices []choice) {
	offerLineChoices(schema, "serviceId", choices)
}

// offerContainers replaces the container field of a line with the list.
func offerContainers(schema map[string]any, choices []choice) {
	offerLineChoices(schema, "containerNo", choices)
}

// offerLineChoices turns one field of a line into a oneOf of choices.
func offerLineChoices(schema map[string]any, name string, choices []choice) {
	props, _ := schema["properties"].(map[string]any)
	lines, _ := props["lines"].(map[string]any)
	items, _ := lines["items"].(map[string]any)
	lineProps, _ := items["properties"].(map[string]any)
	field, _ := lineProps[name].(map[string]any)
	if field == nil {
		// The form no longer has the field this projector exists to fill in;
		// render it as authored rather than invent one.
		return
	}

	options := make([]any, 0, len(choices))
	for _, c := range choices {
		options = append(options, map[string]any{"const": c.value, "title": c.label})
	}
	field["oneOf"] = options
}
