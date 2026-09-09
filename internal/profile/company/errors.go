package company

import "errors"

var ErrCompanyNotFound = errors.New("company not found")

var ErrInvalidCompanyID = errors.New("invalid company ID")

// ErrOUHandleConflict is returned when updating a company's OUHandle to a value already used by
// another company record (ou_handle has a database-level unique constraint).
var ErrOUHandleConflict = errors.New("ou_handle already in use by another company")
