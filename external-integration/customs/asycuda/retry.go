package asycuda

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// taskRetryBackoff is how long a callback whose task was not waiting yet waits
// before each of our own retries: about 19 minutes in all, well past the time a
// workflow takes to park the task after SLC Edge accepted the dispatch.
var taskRetryBackoff = []time.Duration{
	5 * time.Second, 15 * time.Second, 30 * time.Second,
	time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute,
}

// taskRetries re-runs a callback that arrived before the task it answers had
// parked. The callback is still answered 503, so SLC Edge retries it too (§2);
// these retries cover the case where its schedule runs out first. Whichever
// delivery lands first completes the task, and the other then finds a
// duplicate: the services complete the task before they record the callback.
//
// The retries live in this process only, so a restart drops them and leaves
// SLC Edge's retries to deliver the callback.
type taskRetries struct {
	backoff []time.Duration
	// pending holds the key of each callback being retried, so a redelivery
	// answered 503 while one is pending does not start a second retry.
	pending sync.Map
	wg      sync.WaitGroup
}

func newTaskRetries(backoff []time.Duration) *taskRetries {
	return &taskRetries{backoff: backoff}
}

// schedule retries process until it succeeds or returns an error notParked
// does not accept, or the backoff runs out. key names the callback. process
// must treat a callback already handled as success.
func (t *taskRetries) schedule(ctx context.Context, key string, process func(context.Context) error, notParked func(error) bool) {
	if _, running := t.pending.LoadOrStore(key, struct{}{}); running {
		slog.DebugContext(ctx, "slce: retry already pending for callback", "callback", key)
		return
	}
	// The retries outlive the request, but keep its trace and log values.
	ctx = context.WithoutCancel(ctx)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer t.pending.Delete(key)
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(ctx, "slce: retry of callback panicked", "callback", key, "panic", p)
			}
		}()
		for i, wait := range t.backoff {
			time.Sleep(wait)
			err := process(ctx)
			switch {
			case err == nil:
				slog.InfoContext(ctx, "slce: callback processed on retry", "callback", key, "attempt", i+1)
				return
			case notParked(err):
				slog.DebugContext(ctx, "slce: task still not waiting; will retry", "callback", key, "attempt", i+1)
			default:
				slog.WarnContext(ctx, "slce: retry of callback failed; not retrying", "callback", key, "attempt", i+1, "error", err)
				return
			}
		}
		slog.ErrorContext(ctx, "slce: task never parked; gave up retrying callback",
			"callback", key, "attempts", len(t.backoff))
	}()
}

// wait blocks until every scheduled retry has finished.
func (t *taskRetries) wait() { t.wg.Wait() }
