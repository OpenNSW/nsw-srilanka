package authz

import (
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// ExtAuthz is the extension id; it must match the ExtensionConfig.id declared in
// the SubTaskTemplate JSON configs.
const ExtAuthz = "authz"

// Register installs the task-step authorization extension on reg. cat is the
// slice of the global catalog loaded at the composition root; per-task rule names
// resolve through it.
//
// The catalog may not define taskauthz.SystemPrincipal. A rule naming it admits
// this service's own in-process callers, so if the catalog mapped it to a token
// role or a client id, a request could satisfy that rule as well.
func Register(reg *extensions.Registry, cat taskauthz.Catalog) error {
	if reg == nil {
		return fmt.Errorf("authz: registry is nil")
	}
	if len(cat.Roles) == 0 && len(cat.Clients) == 0 {
		return fmt.Errorf("authz: catalog defines no roles or clients")
	}
	if _, ok := cat.Roles[taskauthz.SystemPrincipal]; ok {
		return fmt.Errorf("authz: catalog roles define %q, a name reserved for this service's own callers", taskauthz.SystemPrincipal)
	}
	if _, ok := cat.Clients[taskauthz.SystemPrincipal]; ok {
		return fmt.Errorf("authz: catalog clients define %q, a name reserved for this service's own callers", taskauthz.SystemPrincipal)
	}
	if err := reg.Register(ExtAuthz, NewExtension(cat)); err != nil {
		return fmt.Errorf("authz: register %s: %w", ExtAuthz, err)
	}
	return nil
}
