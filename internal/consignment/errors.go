package consignment

import "errors"

var (
	// ErrConsignmentNotFound is returned when a consignment is not found.
	ErrConsignmentNotFound = errors.New("consignment not found")
	// ErrAccessDenied is returned when the caller's company is neither the trader nor the
	// CHA company of the consignment. Callers should translate it to a 404 (not a 403) so
	// the response cannot be used to distinguish an existing consignment from a missing one.
	ErrAccessDenied = errors.New("access denied")
	// ErrFlowNotAllowed is returned when a caller asks to start a flow that is not one of the
	// consignment's startable flows (Config.Flows).
	ErrFlowNotAllowed = errors.New("flow cannot be started on a consignment")
)
