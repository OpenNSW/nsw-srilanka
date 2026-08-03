package authz

import (
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
)

// ExtAuthz is the extension id; it must match the ExtensionConfig.id declared in
// the SubTaskTemplate JSON configs.
const ExtAuthz = "authz"

// Register installs the task-step authorization extension on reg. cat is the
// slice of the global catalog loaded at the composition root; per-task rule names
// resolve through it.
func Register(reg *extensions.Registry, cat Catalog) error {
	if reg == nil {
		return fmt.Errorf("authz: registry is nil")
	}
	if len(cat.Roles) == 0 && len(cat.Clients) == 0 {
		return fmt.Errorf("authz: catalog defines no roles or clients")
	}
	if err := reg.Register(ExtAuthz, NewExtension(cat)); err != nil {
		return fmt.Errorf("authz: register %s: %w", ExtAuthz, err)
	}
	return nil
}
