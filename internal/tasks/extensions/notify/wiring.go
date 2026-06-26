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

// Register installs the nsw-srilanka task extensions on reg.
//
// The notification extension dispatches SMS/email through notification.Manager
// as a side-effect of a step completing, resolving the recipient from
// accumulated workflow state. s must be non-nil; bootstrap fail-fasts if the
// notification manager could not initialize.
func Register(reg *extensions.Registry, s sender, devMode bool) error {
	if reg == nil {
		return fmt.Errorf("extensions: registry is nil")
	}
	if s == nil {
		return fmt.Errorf("extensions: sender is nil")
	}

	if err := reg.Register(ExtNotification, NewNotificationExtension(s, devMode)); err != nil {
		return fmt.Errorf("extensions: register %s: %w", ExtNotification, err)
	}
	return nil
}
