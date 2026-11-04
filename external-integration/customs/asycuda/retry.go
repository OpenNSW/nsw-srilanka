package asycuda

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

// A callback that arrives before what it answers is there (its declaration or
// dispatch note not recorded yet, or the task not parked) is accepted with a
// 202 and handed to a Temporal workflow, which applies it once it can. SLC Edge
// stops retrying on the 202, so the hand-off is durable: the workflow survives a
// restart, and the 202 is only sent once Temporal has the callback.
const (
	// CallbackQueue is the Temporal task queue the callback retries run on.
	CallbackQueue = "SLCE_CALLBACK_QUEUE"

	callbackWorkflowName = "SLCECallbackRetry"
	callbackActivityName = "ApplySLCECallback"

	// callbackRetryWindow is how long a callback is retried before it is given
	// up: long enough for a callback that raced the integration result of the
	// same document, or a delivery of it SLC Edge retried late, to land first.
	callbackRetryWindow = 24 * time.Hour

	// errTypePermanent marks a callback a retry cannot fix.
	errTypePermanent = "permanent"
)

// callbackRetryPolicy retries a callback after 5s, doubling up to every 10
// minutes, until callbackRetryWindow runs out.
var callbackRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:        5 * time.Second,
	BackoffCoefficient:     2,
	MaximumInterval:        10 * time.Minute,
	NonRetryableErrorTypes: []string{errTypePermanent},
}

// Callback is a callback held for retry: its event type and the body SLC Edge
// sent, applied again exactly as it arrived.
type Callback struct {
	EventType string
	Body      []byte
}

// CallbackScheduler hands a callback to the retry workflow.
type CallbackScheduler interface {
	Schedule(ctx context.Context, cb Callback) error
}

// TemporalCallbacks starts the retry workflow for a callback.
type TemporalCallbacks struct {
	client client.Client
}

// NewTemporalCallbacks returns a scheduler that starts retries through c.
func NewTemporalCallbacks(c client.Client) TemporalCallbacks { return TemporalCallbacks{client: c} }

// Schedule starts the retry workflow for cb. The workflow ID names the callback
// by its body, so a redelivery while a retry is running joins that retry, while
// two different callbacks for the same document never merge.
func (t TemporalCallbacks) Schedule(ctx context.Context, cb Callback) error {
	sum := sha256.Sum256(cb.Body)
	_, err := t.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                       "slce-callback-" + cb.EventType + "-" + hex.EncodeToString(sum[:12]),
		TaskQueue:                CallbackQueue,
		WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
	}, callbackWorkflowName, cb)
	if err != nil {
		return fmt.Errorf("start the retry of %s: %w", cb.EventType, err)
	}
	return nil
}

// NewCallbackWorker returns the worker that runs the callback retries, applying
// each through h's services. The caller starts and stops it.
func NewCallbackWorker(c client.Client, h *Handler) worker.Worker {
	w := worker.New(c, CallbackQueue, worker.Options{})
	w.RegisterWorkflowWithOptions(callbackWorkflow, workflow.RegisterOptions{Name: callbackWorkflowName})
	w.RegisterActivityWithOptions(h.applyCallbackActivity, activityRegisterOptions())
	return w
}

func activityRegisterOptions() activity.RegisterOptions {
	return activity.RegisterOptions{Name: callbackActivityName}
}

// callbackWorkflow applies a callback, retrying it until it lands or the retry
// window runs out.
func callbackWorkflow(ctx workflow.Context, cb Callback) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout:    time.Minute,
		ScheduleToCloseTimeout: callbackRetryWindow,
		RetryPolicy:            callbackRetryPolicy,
	})
	err := workflow.ExecuteActivity(ctx, callbackActivityName, cb).Get(ctx, nil)
	if err != nil {
		workflow.GetLogger(ctx).Error("slce: gave up applying callback", "eventType", cb.EventType, "error", err)
	}
	return err
}

// applyCallbackActivity applies a held callback. A callback still early returns
// a retryable error; one a retry cannot fix is marked permanent.
func (h *Handler) applyCallbackActivity(ctx context.Context, cb Callback) error {
	err := h.applyCallback(ctx, cb.EventType, cb.Body)
	if err != nil && isPermanent(err) {
		return temporal.NewNonRetryableApplicationError(err.Error(), errTypePermanent, err)
	}
	return err
}

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
