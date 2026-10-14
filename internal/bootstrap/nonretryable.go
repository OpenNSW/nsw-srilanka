package bootstrap

import (
	"errors"

	"go.temporal.io/sdk/temporal"
)

// keepNonRetryable returns err as a non-retryable error when a plugin marked it so.
// Temporal reads retryability from the outermost error, and StartTaskStep wraps a
// plugin's error with %w, so without this a plugin's non-retryable verdict is lost
// and the step is retried forever.
func keepNonRetryable(err error) error {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.NonRetryable() {
		return temporal.NewNonRetryableApplicationError(err.Error(), appErr.Type(), err)
	}
	return err
}
