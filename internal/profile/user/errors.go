package user

import (
	"errors"
)

// ErrUserNotFound is returned when a user record is not found in the database.
var ErrUserNotFound = errors.New("user not found")

// ErrInvalidUserID is returned when an invalid user ID is provided.
var ErrInvalidUserID = errors.New("invalid user ID")
