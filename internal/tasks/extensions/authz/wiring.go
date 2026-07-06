package authz

import (
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
)

// ExtAuthz is the extension id; it must match the ExtensionConfig.id declared in
// the SubTaskTemplate JSON configs.
const ExtAuthz = "authz"

// Register loads and validates the catalog at configPath and installs the
// task-step authorization extension on reg.
func Register(reg *extensions.Registry, configPath string) error {
	if reg == nil {
		return fmt.Errorf("authz: registry is nil")
	}
	catalog, err := LoadCatalog(configPath)
	if err != nil {
		return err
	}
	if err := reg.Register(ExtAuthz, NewExtension(catalog)); err != nil {
		return fmt.Errorf("authz: register %s: %w", ExtAuthz, err)
	}
	return nil
}
