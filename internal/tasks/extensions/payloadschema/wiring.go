package payloadschema

import (
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
)

// ExtPayloadSchema is the extension id; it must match the ExtensionConfig.id
// declared in the step template JSON configs.
const ExtPayloadSchema = "payload_schema"

// Register installs the payload schema validation extension on reg.
// formOf reads forms from the artifacts, namespaceOf the output namespace of a
// step template, and fileOf the files storage holds.
func Register(reg *extensions.Registry, formOf FormOf, namespaceOf NamespaceOf, fileOf FileOf) error {
	if reg == nil {
		return fmt.Errorf("payload schema: registry is nil")
	}
	if formOf == nil || namespaceOf == nil || fileOf == nil {
		return fmt.Errorf("payload schema: form, namespace and file lookups are required")
	}
	if err := reg.Register(ExtPayloadSchema, NewExtension(formOf, namespaceOf, fileOf)); err != nil {
		return fmt.Errorf("payload schema: register %s: %w", ExtPayloadSchema, err)
	}
	return nil
}
