package asycuda

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

// A callback that arrives before what it answers is there (its declaration or
// dispatch note not recorded yet, or the task not parked) is accepted with a
// 202 and retried in the background until it lands. SLC Edge stops retrying on
// the 202, so these retries are what deliver it. They live in this process
// only: a restart drops the ones still pending.

// callbackRetryTiming is how a held callback is retried: first after Initial,
// doubling up to every Max, until Window has passed since it was held.
type callbackRetryTiming struct {
	Initial, Max, Window time.Duration
}

// defaultCallbackRetryTiming gives a callback that raced the integration result
// of its document, or whose task was slow to park, a day to land.
var defaultCallbackRetryTiming = callbackRetryTiming{
	Initial: 5 * time.Second,
	Max:     10 * time.Minute,
	Window:  24 * time.Hour,
}

// Callback is a callback held for retry: its event type and the body SLC Edge
// sent, applied again exactly as it arrived.
type Callback struct {
	EventType string
	Body      []byte
}

// callbackScheduler holds a callback for retry.
type callbackScheduler interface {
	schedule(ctx context.Context, cb Callback)
}

// callbackRetries retries each held callback in a goroutine of its own.
type callbackRetries struct {
	apply  func(context.Context, string, []byte) error
	timing callbackRetryTiming
	// pending holds the key of each callback being retried, so a redelivery
	// while one is pending does not start a second retry.
	pending sync.Map
	wg      sync.WaitGroup
}

func newCallbackRetries(apply func(context.Context, string, []byte) error, timing callbackRetryTiming) *callbackRetries {
	return &callbackRetries{apply: apply, timing: timing}
}

// schedule starts retrying cb. A callback is named by its body, so an identical
// redelivery while it is pending joins that retry, while two different callbacks
// for the same document are retried apart.
func (c *callbackRetries) schedule(ctx context.Context, cb Callback) {
	sum := sha256.Sum256(cb.Body)
	key := cb.EventType + " " + hex.EncodeToString(sum[:12])
	if _, running := c.pending.LoadOrStore(key, struct{}{}); running {
		slog.DebugContext(ctx, "slce: callback already held for retry", "callback", key)
		return
	}
	// The retries outlive the request, but keep its trace and log values.
	ctx = context.WithoutCancel(ctx)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer c.pending.Delete(key)
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(ctx, "slce: retry of held callback panicked", "callback", key, "panic", p)
			}
		}()
		c.retry(ctx, key, cb)
	}()
}

func (c *callbackRetries) retry(ctx context.Context, key string, cb Callback) {
	giveUpAt := time.Now().Add(c.timing.Window)
	wait := c.timing.Initial
	for attempt := 1; ; attempt++ {
		time.Sleep(wait)
		err := c.apply(ctx, cb.EventType, cb.Body)
		switch {
		case err == nil:
			slog.InfoContext(ctx, "slce: held callback applied", "callback", key, "attempt", attempt)
			return
		case isPermanent(err):
			slog.ErrorContext(ctx, "slce: held callback cannot be applied; giving up",
				"callback", key, "attempt", attempt, "error", err)
			return
		}
		wait = min(2*wait, c.timing.Max)
		if time.Now().Add(wait).After(giveUpAt) {
			slog.ErrorContext(ctx, "slce: held callback still not applied; giving up",
				"callback", key, "attempts", attempt, "error", err)
			return
		}
		slog.DebugContext(ctx, "slce: held callback not applied yet; will retry",
			"callback", key, "attempt", attempt, "retryIn", wait, "error", err)
	}
}

// wait blocks until every held callback has been applied or given up.
func (c *callbackRetries) wait() { c.wg.Wait() }

// errInvalidCallback is a held callback that no longer decodes or validates.
var errInvalidCallback = errors.New("invalid callback")

// isPermanent reports whether err is one a retry cannot fix. Anything else,
// whether the callback is still early or a read failed, is retried.
func isPermanent(err error) bool {
	for _, target := range []error{
		errInvalidCallback,
		cusdec.ErrWorkflowNotFoundByEdgeID, cusdec.ErrCaseNotRecorded,
		cdn.ErrDispatchNoteNotFoundByEdgeID, cdn.ErrCaseNotRecorded,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// isEarly reports whether err means the callback arrived before what it answers
// was there, so it is held for retry rather than refused.
func isEarly(err error) bool {
	for _, target := range []error{
		cusdec.ErrDeclarationNotRecorded, cusdec.ErrTaskNotParked, cusdec.ErrCusdecNotFoundByRef,
		cdn.ErrDispatchNoteNotRecorded, cdn.ErrTaskNotParked, cdn.ErrDispatchNoteNotFoundByCDNRef,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// applyCallback runs a held callback through the service the handler would
// have, treating one already applied as done.
func (h *Handler) applyCallback(ctx context.Context, eventType string, body []byte) error {
	appliers := map[string]func(context.Context, []byte) error{
		"CUSDEC_INTEGRATED": func(ctx context.Context, body []byte) error {
			return applyAs(ctx, body, h.cusdecService.ProcessIntegrationResult, cusdec.ErrDuplicateIntegrationResult)
		},
		"PAYMENT_CONFIRMED":    h.cusdecEventApplier("PAYMENT_CONFIRMED"),
		"WARRANTING_COMPLETED": h.cusdecEventApplier("WARRANTING_COMPLETED"),
		"EXPORT_RELEASED":      h.cusdecEventApplier("EXPORT_RELEASED"),
		cusdec.EventCusdecAmended: func(ctx context.Context, body []byte) error {
			return applyAs(ctx, body, h.cusdecService.ProcessAmendment, nil)
		},
		cusdec.EventCusdecCancelled: func(ctx context.Context, body []byte) error {
			return applyAs(ctx, body, h.cusdecService.ProcessCancellation, nil)
		},
		"CDN_INTEGRATED": func(ctx context.Context, body []byte) error {
			return applyAs(ctx, body, h.cdnService.ProcessIntegrationResult, nil)
		},
		"CDN_ACKNOWLEDGED": func(ctx context.Context, body []byte) error {
			return applyAs(ctx, body, h.cdnService.ProcessAcknowledgment, nil)
		},
		cdn.EventCDNAmended: func(ctx context.Context, body []byte) error {
			return applyAs(ctx, body, h.cdnService.ProcessAmendment, nil)
		},
		cdn.EventCDNCancelled: func(ctx context.Context, body []byte) error {
			return applyAs(ctx, body, h.cdnService.ProcessCancellation, nil)
		},
	}
	apply, ok := appliers[eventType]
	if !ok {
		return fmt.Errorf("%w: event type %q", errInvalidCallback, eventType)
	}
	return apply(ctx, body)
}

// cusdecEventApplier applies a §6.5 event, which, as in the handler, takes its
// type from the route rather than the body.
func (h *Handler) cusdecEventApplier(event string) func(context.Context, []byte) error {
	return func(ctx context.Context, body []byte) error {
		var req cusdec.CusdecEventRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return fmt.Errorf("%w: %w", errInvalidCallback, err)
		}
		req.Event = event
		if err := req.Validate(); err != nil {
			return fmt.Errorf("%w: %w", errInvalidCallback, err)
		}
		return ignore(h.cusdecService.ProcessEvent(ctx, req), cusdec.ErrDuplicateEvent)
	}
}

// applyAs decodes body as T and runs process on it, treating done, a callback
// already applied, as success.
func applyAs[T interface{ Validate() error }](ctx context.Context, body []byte, process func(context.Context, T) error, done error) error {
	req, err := decodeCallback[T](body)
	if err != nil {
		return err
	}
	return ignore(process(ctx, req), done)
}

// decodeCallback decodes and validates a held callback's body.
func decodeCallback[T interface{ Validate() error }](body []byte) (T, error) {
	var req T
	if err := json.Unmarshal(body, &req); err != nil {
		return req, fmt.Errorf("%w: %w", errInvalidCallback, err)
	}
	if err := req.Validate(); err != nil {
		return req, fmt.Errorf("%w: %w", errInvalidCallback, err)
	}
	return req, nil
}

// ignore treats done, a callback already applied, as success. A nil done
// ignores nothing.
func ignore(err, done error) error {
	if done != nil && errors.Is(err, done) {
		return nil
	}
	return err
}
