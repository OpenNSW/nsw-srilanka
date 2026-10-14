package bootstrap

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// nonRetryable reports how Temporal classifies err when an activity returns it.
func nonRetryable(err error) bool {
	return temporal.GetDefaultFailureConverter().ErrorToFailure(err).GetApplicationFailureInfo().GetNonRetryable()
}

func TestKeepNonRetryable(t *testing.T) {
	// StartTaskStep wraps what a plugin returns with %w.
	wrap := func(err error) error {
		return fmt.Errorf("[StartTaskStep] plugin for plugin type %q execution failed: %w", "EXTERNAL_REVIEW", err)
	}

	t.Run("a wrapped non-retryable error stays non-retryable", func(t *testing.T) {
		stale := temporal.NewNonRetryableApplicationError("its step is no longer waiting", "StaleReplyToken", errors.New("remote error (409)"))
		wrapped := wrap(stale)
		require.False(t, nonRetryable(wrapped), "the wrap alone hides the plugin's verdict from Temporal")

		got := keepNonRetryable(wrapped)

		assert.True(t, nonRetryable(got))
		var appErr *temporal.ApplicationError
		require.ErrorAs(t, got, &appErr)
		assert.Equal(t, "StaleReplyToken", appErr.Type())
		assert.ErrorIs(t, got, stale)
	})

	t.Run("a wrapped retryable error is returned as it is", func(t *testing.T) {
		wrapped := wrap(errors.New("remote error (503)"))

		got := keepNonRetryable(wrapped)

		assert.Equal(t, wrapped, got)
		assert.False(t, nonRetryable(got))
	})

	t.Run("nil and a pending result pass through", func(t *testing.T) {
		assert.NoError(t, keepNonRetryable(nil))
		assert.Equal(t, activity.ErrResultPending, keepNonRetryable(activity.ErrResultPending))
	})
}
