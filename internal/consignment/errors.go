package consignment

import "errors"

var (
	// ErrCompanyNotCHA is returned when a trader selects a company that is not enabled as a CHA company.
	ErrCompanyNotCHA = errors.New("company is not enabled as a CHA")

	// ErrCHACompanyMismatch is returned at Stage 2 when the CHA attempting to claim a
	// consignment does not belong to the CHA company chosen by the trader at Stage 1.
	ErrCHACompanyMismatch = errors.New("CHA does not belong to the consignment's CHA company")

	// ErrConsignmentNotFound is returned when a consignment is not found.
	ErrConsignmentNotFound = errors.New("consignment not found")
)
