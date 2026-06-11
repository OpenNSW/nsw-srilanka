package company

import "errors"

var ErrCompanyNotFound = errors.New("company not found")

var ErrInvalidCompanyID = errors.New("invalid company ID")
