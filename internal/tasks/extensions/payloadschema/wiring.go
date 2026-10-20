package payloadschema

import (
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
)

// ExtPayloadSchema is the extension id; it must match the ExtensionConfig.id
// declared in the step template JSON configs.
const ExtPayloadSchema = "payload_schema"

// Register installs the payload schema validation extension on reg.
// stepTemplateOf and formOf read step templates and forms from the artifacts.
func Register(reg *extensions.Registry, stepTemplateOf StepTemplateOf, formOf FormOf) error {
	if reg == nil {
		return fmt.Errorf("payload schema: registry is nil")
	}
	if stepTemplateOf == nil || formOf == nil {
		return fmt.Errorf("payload schema: step template and form lookups are required")
	}
	if err := reg.Register(ExtPayloadSchema, NewExtension(stepTemplateOf, formOf)); err != nil {
		return fmt.Errorf("payload schema: register %s: %w", ExtPayloadSchema, err)
	}
	return nil
}
