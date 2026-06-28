package notify

import (
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
)

// Extension ids. These must match the ExtensionConfig.id values declared in the
// SubTaskTemplate JSON configs loaded into the artifact registry.
const (
	ExtNotification = "notification"
)

// Register installs the nsw-srilanka task extensions on reg. The notification
// extension dispatches SMS/email as a side-effect of a step completing. s and
// loader must be non-nil; loader resolves template_id documents.
func Register(reg *extensions.Registry, s sender, loader templateLoader, devMode bool) error {
	if reg == nil {
		return fmt.Errorf("extensions: registry is nil")
	}
	if s == nil {
		return fmt.Errorf("extensions: sender is nil")
	}
	if loader == nil {
		return fmt.Errorf("extensions: template loader is nil")
	}

	if err := reg.Register(ExtNotification, NewNotificationExtension(s, loader, devMode)); err != nil {
		return fmt.Errorf("extensions: register %s: %w", ExtNotification, err)
	}
	return nil
}
