package renderer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/uiprojector"
)

// ProjectorSLPAConsolidation defines the container-consolidation form projector.
const ProjectorSLPAConsolidation uiprojector.ProjectorType = "SLPA_CONSOLIDATION"

// SLPAConsolidationProjector renders one container's consolidation form with the
// real containers available to pair as a list to pick from.
//
// Which containers those are belongs to the consignment and to the moment: they
// are the ones the terminal has pre-advised in Navis against this declaration
// and SLPA has not already consolidated, so a schema authored ahead of time
// cannot enumerate them. Left as free text the trader would copy a number by eye
// into a field beside it, and a typo becomes a save the CMS refuses for reasons
// that read as if the container were at fault.
//
// An empty list is a real state, not an error: the container has not been
// pre-advised yet. The trader does that in Navis and comes back to this same
// task, which is why the branch waits rather than failing.
type SLPAConsolidationProjector struct{}

// NewSLPAConsolidationProjector creates a new SLPAConsolidationProjector.
func NewSLPAConsolidationProjector() *SLPAConsolidationProjector {
	return &SLPAConsolidationProjector{}
}

// Type returns the projector type.
func (p *SLPAConsolidationProjector) Type() uiprojector.ProjectorType {
	return ProjectorSLPAConsolidation
}

// consolidationNamespace is where the lookup records what it found, and
// formNamespace is what the trader's form edits.
const (
	consolidationNamespace = "consolidation"
	formNamespace          = "consolidationform"
)

// Project renders the form artifact with the container field turned into a list
// of the real containers this declaration currently has available.
//
// With nothing to offer the form is rendered as authored: an enum with no
// members is a field nothing can satisfy, which would leave the trader unable to
// submit and unable to see why. The step's own panel says what SLPA is holding.
func (p *SLPAConsolidationProjector) Project(_ context.Context, templateContent []byte, data any) (uiprojector.Projection, error) {
	var form struct {
		Schema   map[string]any `json:"schema"`
		UISchema map[string]any `json:"uiSchema"`
	}
	if err := json.Unmarshal(templateContent, &form); err != nil {
		return uiprojector.Projection{}, fmt.Errorf("slpa_consolidation_projector: parse form: %w", err)
	}

	record, _ := data.(map[string]any)
	if choices := availableContainers(record); len(choices) > 0 {
		offerChoices(form.Schema, choices)
	}

	return uiprojector.Projection{
		Type: uiprojector.SectionTypeForm,
		Content: uiprojector.FormContent{
			Schema:   form.Schema,
			UISchema: form.UISchema,
			Data:     record[formNamespace],
		},
	}, nil
}

// choice is one option in the list: what the trader reads, and what is sent.
type choice struct{ value, label string }

// availableContainers reads the real containers still free to pair.
//
// The value is SLPA's sqid rather than the container number, because the same
// answer keys the two calls that follow: the save pairs by it, and the delete
// endpoint is addressed by it. The trader still reads the container number,
// which is what is written on the box in front of them.
//
// A container already consolidated is left out — pairing it again is a save the
// CMS refuses — and so is an entry missing either half, which would otherwise
// become an option nothing downstream could resolve.
func availableContainers(record map[string]any) []choice {
	ns, _ := record[consolidationNamespace].(map[string]any)
	raw, _ := ns["cap_containers"].([]any)

	out := make([]choice, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if paired, _ := row["so_container_sqid"].(string); paired != "" {
			continue
		}
		sqid, _ := row["sqid"].(string)
		number, _ := row["container_no"].(string)
		if sqid == "" || number == "" {
			continue
		}
		out = append(out, choice{value: sqid, label: number})
	}
	return out
}

// offerChoices replaces the container field's free text with a fixed list.
//
// oneOf rather than enum, because JSONForms renders a oneOf of const/title pairs
// as a select while a bare enum depends on the renderer set in use; the titles
// are the numbers themselves, which is what the trader matches against the
// container in front of them.
func offerChoices(schema map[string]any, choices []choice) {
	props, _ := schema["properties"].(map[string]any)
	field, _ := props["cap_container_no"].(map[string]any)
	if field == nil {
		// The form no longer has the field this projector exists to fill in.
		// Rendering it as authored is the honest outcome: whoever changed the
		// artifact meant something, and inventing a field here would hide it.
		return
	}

	options := make([]any, 0, len(choices))
	for _, c := range choices {
		options = append(options, map[string]any{"const": c.value, "title": c.label})
	}
	field["oneOf"] = options
}
