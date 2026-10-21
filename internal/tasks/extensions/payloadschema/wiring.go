package payloadschema

import (
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
)

// ExtPayloadSchema is the extension id; it must match the ExtensionConfig.id
// declared in the step template JSON configs.
const ExtPayloadSchema = "payload_schema"

// Register installs the payload schema validation extension on reg.
// formOf reads forms from the artifacts.
func Register(reg *extensions.Registry, formOf FormOf) error {
	if reg == nil {
		return fmt.Errorf("payload schema: registry is nil")
	}
	if formOf == nil {
		return fmt.Errorf("payload schema: form lookup is required")
	}
	if err := reg.Register(ExtPayloadSchema, NewExtension(formOf)); err != nil {
		return fmt.Errorf("payload schema: register %s: %w", ExtPayloadSchema, err)
	}
	return nil
}
