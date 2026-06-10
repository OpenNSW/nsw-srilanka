package consignment

import "errors"

var (
	// ErrConsignmentNotFound is returned when a consignment is not found.
	ErrConsignmentNotFound = errors.New("consignment not found")
)
