package cdn

import "errors"

// ErrDispatchNoteNotFoundByEdgeID indicates no dispatch note matches the given edgeId.
// This is a permanent condition — the caller should not retry.
var ErrDispatchNoteNotFoundByEdgeID = errors.New("dispatch note not found by edgeId")

// ErrDispatchNoteNotFoundByCDNRef indicates no dispatch note matches the given cdnRef.
// This can be a transient condition if the acknowledgment callback arrives before
// the integration result callback has finished processing — the caller should retry.
var ErrDispatchNoteNotFoundByCDNRef = errors.New("dispatch note not found by cdnRef")

// ErrInvalidCallbackPayload indicates the ASYCUDA callback payload was
// malformed, missing required fields, or otherwise failed validation.
var ErrInvalidCallbackPayload = errors.New("invalid callback payload")

// ErrTaskNotParkedYet indicates the workflow exists but the step this callback
// answers had not parked when it arrived — the callback won a race with the
// submission it belongs to.
//
// Transient, and the caller should retry: §2 has SLC Edge redeliver up to four
// times, and the next delivery finds the task parked. Acknowledging it instead
// strands the task on a result that has already been and gone, which is not
// recoverable without someone replaying the callback by hand.
var ErrTaskNotParkedYet = errors.New("task not parked yet for this callback")
